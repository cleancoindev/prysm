package blockchain

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"
	"github.com/prysmaticlabs/prysm/shared/hashutil"
	"github.com/prysmaticlabs/prysm/shared/params"
	"go.opencensus.io/trace"
)

var (
	reorgCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reorg_counter",
		Help: "The number of chain reorganization events that have happened in the fork choice rule",
	})
)

// ForkChoice interface defines the methods for applying fork choice rule
// operations to the blockchain.
type ForkChoice interface {
	ApplyForkChoiceRule(ctx context.Context, block *pb.BeaconBlock, computedState *pb.BeaconState) error
}

// updateFFGCheckPts checks whether the existing FFG check points saved in DB
// are not older than the ones just processed in state. If it's older, we update
// the db with the latest FFG check points, both justification and finalization.
func (c *ChainService) updateFFGCheckPts(ctx context.Context, state *pb.BeaconState) error {
	lastJustifiedSlot := helpers.StartSlot(state.JustifiedEpoch)
	savedJustifiedBlock, err := c.beaconDB.JustifiedBlock()
	if err != nil {
		return err
	}
	// If the last processed justification slot in state is greater than
	// the slot of justified block saved in DB.
	if lastJustifiedSlot > savedJustifiedBlock.Slot {
		// Retrieve the new justified block from DB using the new justified slot and save it.
		newJustifiedBlock, err := c.beaconDB.BlockBySlot(ctx, lastJustifiedSlot)
		if err != nil {
			return err
		}
		// If the new justified slot is a skip slot in db then we keep getting it's ancestors
		// until we can get a block.
		lastAvailBlkSlot := lastJustifiedSlot
		for newJustifiedBlock == nil {
			log.Debugf("Saving new justified block, no block with slot %d in db, trying slot %d",
				lastAvailBlkSlot, lastAvailBlkSlot-1)
			lastAvailBlkSlot--
			newJustifiedBlock, err = c.beaconDB.BlockBySlot(ctx, lastAvailBlkSlot)
			if err != nil {
				return err
			}
		}

		// Fetch justified state from historical states db.
		newJustifiedState, err := c.beaconDB.HistoricalStateFromSlot(ctx, newJustifiedBlock.Slot)
		if err != nil {
			return err
		}
		if err := c.beaconDB.SaveJustifiedBlock(newJustifiedBlock); err != nil {
			return err
		}
		if err := c.beaconDB.SaveJustifiedState(newJustifiedState); err != nil {
			return err
		}
	}

	lastFinalizedSlot := helpers.StartSlot(state.FinalizedEpoch)
	savedFinalizedBlock, err := c.beaconDB.FinalizedBlock()
	// If the last processed finalized slot in state is greater than
	// the slot of finalized block saved in DB.
	if err != nil {
		return err
	}
	if lastFinalizedSlot > savedFinalizedBlock.Slot {
		// Retrieve the new finalized block from DB using the new finalized slot and save it.
		newFinalizedBlock, err := c.beaconDB.BlockBySlot(ctx, lastFinalizedSlot)
		if err != nil {
			return err
		}
		// If the new finalized slot is a skip slot in db then we keep getting it's ancestors
		// until we can get a block.
		lastAvailBlkSlot := lastFinalizedSlot
		for newFinalizedBlock == nil {
			log.Debugf("Saving new finalized block, no block with slot %d in db, trying slot %d",
				lastAvailBlkSlot, lastAvailBlkSlot-1)
			lastAvailBlkSlot--
			newFinalizedBlock, err = c.beaconDB.BlockBySlot(ctx, lastAvailBlkSlot)
			if err != nil {
				return err
			}
		}

		// Generate the new finalized state with using new finalized block and
		// save it.
		newFinalizedState, err := c.beaconDB.HistoricalStateFromSlot(ctx, lastFinalizedSlot)
		if err != nil {
			return err
		}
		if err := c.beaconDB.SaveFinalizedBlock(newFinalizedBlock); err != nil {
			return err
		}
		if err := c.beaconDB.SaveFinalizedState(newFinalizedState); err != nil {
			return err
		}
	}
	return nil
}

// ApplyForkChoiceRule determines the current beacon chain head using LMD
// GHOST as a block-vote weighted function to select a canonical head in
// Ethereum Serenity. The inputs are the the recently processed block and its
// associated state.
func (c *ChainService) ApplyForkChoiceRule(
	ctx context.Context,
	block *pb.BeaconBlock,
	postState *pb.BeaconState,
) error {
	ctx, span := trace.StartSpan(ctx, "beacon-chain.blockchain.ApplyForkChoiceRule")
	defer span.End()
	log.Info("Applying LMD-GHOST Fork Choice Rule")

	justifiedState, err := c.beaconDB.JustifiedState()
	if err != nil {
		return fmt.Errorf("could not retrieve justified state: %v", err)
	}
	attestationTargets, err := c.attestationTargets(ctx, justifiedState)
	if err != nil {
		return fmt.Errorf("could not retrieve attestation target: %v", err)
	}
	justifiedHead, err := c.beaconDB.JustifiedBlock()
	if err != nil {
		return fmt.Errorf("could not retrieve justified head: %v", err)
	}
	head, err := c.lmdGhost(ctx, justifiedHead, justifiedState, attestationTargets)
	if err != nil {
		return fmt.Errorf("could not run fork choice: %v", err)
	}
	headRoot, err := hashutil.HashBeaconBlock(head)
	if err != nil {
		return fmt.Errorf("could not hash head block: %v", err)
	}
	c.canonicalBlocksLock.Lock()
	defer c.canonicalBlocksLock.Unlock()
	c.canonicalBlocks[head.Slot] = headRoot[:]

	newState := postState
	if head.Slot != block.Slot {
		log.Warnf("Reorg happened, last processed block at slot %d, new head block at slot %d",
			block.Slot-params.BeaconConfig().GenesisSlot, head.Slot-params.BeaconConfig().GenesisSlot)

		// Only regenerate head state if there was a reorg.
		newState, err = c.beaconDB.HistoricalStateFromSlot(ctx, head.Slot)
		if err != nil {
			return fmt.Errorf("could not gen state: %v", err)
		}

		if newState.Slot != postState.Slot {
			log.Warnf("Reorg happened, post state slot at %d, new head state at slot %d",
				postState.Slot-params.BeaconConfig().GenesisSlot, newState.Slot-params.BeaconConfig().GenesisSlot)
		}

		for revertedSlot := block.Slot; revertedSlot > head.Slot; revertedSlot-- {
			delete(c.canonicalBlocks, revertedSlot)
		}
		reorgCount.Inc()
	}

	if err := c.beaconDB.UpdateChainHead(ctx, head, newState); err != nil {
		return fmt.Errorf("failed to update chain: %v", err)
	}
	h, err := hashutil.HashBeaconBlock(head)
	if err != nil {
		return fmt.Errorf("could not hash head: %v", err)
	}
	log.WithField("headRoot", fmt.Sprintf("0x%x", h)).Info("Chain head block and state updated")
	return nil
}

// lmdGhost applies the Latest Message Driven, Greediest Heaviest Observed Sub-Tree
// fork-choice rule defined in the Ethereum Serenity specification for the beacon chain.
//
// Spec pseudocode definition:
//	def lmd_ghost(store: Store, start_state: BeaconState, start_block: BeaconBlock) -> BeaconBlock:
//    """
//    Execute the LMD-GHOST algorithm to find the head ``BeaconBlock``.
//    """
//    validators = start_state.validator_registry
//    active_validator_indices = get_active_validator_indices(validators, slot_to_epoch(start_state.slot))
//    attestation_targets = [
//        (validator_index, get_latest_attestation_target(store, validator_index))
//        for validator_index in active_validator_indices
//    ]
//
//    def get_vote_count(block: BeaconBlock) -> int:
//        return sum(
//            get_effective_balance(start_state.validator_balances[validator_index]) // FORK_CHOICE_BALANCE_INCREMENT
//            for validator_index, target in attestation_targets
//            if get_ancestor(store, target, block.slot) == block
//        )
//
//    head = start_block
//    while 1:
//        children = get_children(store, head)
//        if len(children) == 0:
//            return head
//        head = max(children, key=get_vote_count)
func (c *ChainService) lmdGhost(
	ctx context.Context,
	startBlock *pb.BeaconBlock,
	startState *pb.BeaconState,
	voteTargets map[uint64]*pb.BeaconBlock,
) (*pb.BeaconBlock, error) {
	highestSlot := c.beaconDB.HighestBlockSlot()

	head := startBlock
	for {
		children, err := c.blockChildren(ctx, head, highestSlot)
		if err != nil {
			return nil, fmt.Errorf("could not fetch block children: %v", err)
		}
		if len(children) == 0 {
			return head, nil
		}
		maxChild := children[0]

		maxChildVotes, err := VoteCount(maxChild, startState, voteTargets, c.beaconDB)
		if err != nil {
			return nil, fmt.Errorf("unable to determine vote count for block: %v", err)
		}
		for i := 0; i < len(children); i++ {
			candidateChildVotes, err := VoteCount(children[i], startState, voteTargets, c.beaconDB)
			if err != nil {
				return nil, fmt.Errorf("unable to determine vote count for block: %v", err)
			}
			if candidateChildVotes > maxChildVotes {
				maxChild = children[i]
			}
		}
		head = maxChild
	}
}

// blockChildren returns the child blocks of the given block up to a given
// highest slot.
//
// ex:
//       /- C - E
// A - B - D - F
//       \- G
// Input: B. Output: [C, D, G]
//
// Spec pseudocode definition:
//	get_children(store: Store, block: BeaconBlock) -> List[BeaconBlock]
//		returns the child blocks of the given block.
func (c *ChainService) blockChildren(ctx context.Context, block *pb.BeaconBlock, highestSlot uint64) ([]*pb.BeaconBlock, error) {
	var children []*pb.BeaconBlock

	currentRoot, err := hashutil.HashBeaconBlock(block)
	if err != nil {
		return nil, fmt.Errorf("could not tree hash incoming block: %v", err)
	}
	startSlot := block.Slot + 1
	for i := startSlot; i <= highestSlot; i++ {
		block, err := c.beaconDB.BlockBySlot(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("could not get block by slot: %v", err)
		}
		// Continue if there's a skip block.
		if block == nil {
			continue
		}

		parentRoot := bytesutil.ToBytes32(block.ParentRootHash32)
		if currentRoot == parentRoot {
			children = append(children, block)
		}
	}
	return children, nil
}

// attestationTargets retrieves the list of attestation targets since last finalized epoch,
// each attestation target consists of validator index and its attestation target (i.e. the block
// which the validator attested to)
func (c *ChainService) attestationTargets(ctx context.Context, state *pb.BeaconState) (map[uint64]*pb.BeaconBlock, error) {
	indices := helpers.ActiveValidatorIndices(state.ValidatorRegistry, helpers.CurrentEpoch(state))
	attestationTargets := make(map[uint64]*pb.BeaconBlock)
	for i, index := range indices {
		block, err := c.attsService.LatestAttestationTarget(ctx, index)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve attestation target: %v", err)
		}
		if block == nil {
			continue
		}
		attestationTargets[uint64(i)] = block
	}
	return attestationTargets, nil
}

// VoteCount determines the number of votes on a beacon block by counting the number
// of target blocks that have such beacon block as a common ancestor.
//
// Spec pseudocode definition:
//  def get_vote_count(block: BeaconBlock) -> int:
//        return sum(
//            get_effective_balance(start_state.validator_balances[validator_index]) // FORK_CHOICE_BALANCE_INCREMENT
//            for validator_index, target in attestation_targets
//            if get_ancestor(store, target, block.slot) == block
//        )
func VoteCount(block *pb.BeaconBlock, state *pb.BeaconState, targets map[uint64]*pb.BeaconBlock, beaconDB *db.BeaconDB) (int, error) {
	balances := 0
	for validatorIndex, targetBlock := range targets {
		ancestor, err := BlockAncestor(targetBlock, block.Slot, beaconDB)
		if err != nil {
			return 0, err
		}
		// This covers the following case, we start at B5, and want to process B6 and B7
		// B6 can be processed, B7 can not be processed because it's pointed to the
		// block older than current block 5.
		// B4 - B5 - B6
		//   \ - - - - - B7
		if ancestor == nil {
			continue
		}
		ancestorRoot, err := hashutil.HashBeaconBlock(ancestor)
		if err != nil {
			return 0, err
		}
		blockRoot, err := hashutil.HashBeaconBlock(block)
		if err != nil {
			return 0, err
		}
		if blockRoot == ancestorRoot {
			balances += int(helpers.EffectiveBalance(state, validatorIndex))
		}
	}
	return balances, nil
}

// BlockAncestor obtains the ancestor at of a block at a certain slot.
//
// Spec pseudocode definition:
//  def get_ancestor(store: Store, block: BeaconBlock, slot: Slot) -> BeaconBlock:
//    """
//    Get the ancestor of ``block`` with slot number ``slot``; return ``None`` if not found.
//    """
//    if block.slot == slot:
//        return block
//    elif block.slot < slot:
//        return None
//    else:
//        return get_ancestor(store, store.get_parent(block), slot)
func BlockAncestor(block *pb.BeaconBlock, slot uint64, beaconDB *db.BeaconDB) (*pb.BeaconBlock, error) {
	if block.Slot == slot {
		return block, nil
	}
	if block.Slot < slot {
		return nil, nil
	}
	parentHash := bytesutil.ToBytes32(block.ParentRootHash32)
	parent, err := beaconDB.Block(parentHash)
	if err != nil {
		return nil, fmt.Errorf("could not get parent block: %v", err)
	}
	if parent == nil {
		return nil, fmt.Errorf("parent block does not exist: %v", err)
	}
	return BlockAncestor(parent, slot, beaconDB)
}
