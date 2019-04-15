package blockchain

import (
	"context"
	b "github.com/prysmaticlabs/prysm/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/beacon-chain/internal"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/bitutil"
	"github.com/prysmaticlabs/prysm/shared/hashutil"
	"github.com/prysmaticlabs/prysm/shared/params"
	"testing"
	"time"
)

type assignment struct {
	shard uint64
	validatorIndex uint64
	committee []uint64
}

func createBlock(
	t *testing.T, slot uint64, blocks []*pb.BeaconBlock, states []*pb.BeaconState, assignments map[uint64]*assignment,
) *pb.BeaconBlock {
	if slot % params.BeaconConfig().SlotsPerEpoch == 0 {
		for idx := range states[slot-1].ValidatorRegistry {
			committee, shard, slot, _, err :=
				helpers.CommitteeAssignment(states[slot-1], params.BeaconConfig().GenesisSlot+slot, uint64(idx), false)
			if err != nil {
				t.Fatal(err)
			}
			assignments[slot] = &assignment{
				shard,
				uint64(idx),
				committee,
			}
		}
	}
	parent := blocks[slot-1]
	prevBlockRoot, err := hashutil.HashBeaconBlock(parent)
	if err != nil {
		t.Fatal(err)
	}
	block := &pb.BeaconBlock{
		Slot:             params.BeaconConfig().GenesisSlot + slot,
		RandaoReveal:     []byte{},
		ParentRootHash32: prevBlockRoot[:],
		StateRootHash32:  []byte{},
		Eth1Data: &pb.Eth1Data{
			DepositRootHash32: []byte{},
			BlockHash32:       []byte{},
		},
		Body: &pb.BeaconBlockBody{
			Attestations:      []*pb.Attestation{},
		},
	}

	// We generate attestation using the previous slot due to the MIN_ATTESTATION_INCLUSION_DELAY.
	prevSlot := params.BeaconConfig().GenesisSlot + slot-1
	committee := assignments[prevSlot].committee
	shard := assignments[prevSlot].shard
	attestation := &pb.Attestation{
		Data: &pb.AttestationData{},
	}
	attestation.CustodyBitfield = make([]byte, len(committee))
	// Find the index in committee to be used for the aggregation bitfield.
	var indexInCommittee int
	for j, vIndex := range committee {
		if vIndex == assignments[prevSlot].validatorIndex {
			indexInCommittee = j
			break
		}
	}
	aggregationBitfield := bitutil.SetBitfield(indexInCommittee, len(committee))
	attestation.AggregationBitfield = aggregationBitfield
	attestation.AggregateSignature = []byte("signed")

	epochBoundaryRoot := make([]byte, 32)
	epochStartSlot := helpers.StartSlot(helpers.SlotToEpoch(prevSlot))
	if epochStartSlot == prevSlot {
		epochBoundaryRoot = prevBlockRoot[:]
	} else {
		epochBoundaryRoot, err = b.BlockRoot(states[slot-1], epochStartSlot)
		if err != nil {
			t.Fatal(err)
		}
	}
	// epoch_start_slot = get_epoch_start_slot(slot_to_epoch(head.slot))
	// Fetch the justified block root = hash_tree_root(justified_block) where
	// justified_block is the block at state.justified_epoch in the chain defined by head.
	// On the server side, this is fetched by calling get_block_root(state, justified_epoch).
	// If the last justified boundary slot is the same as state current slot (ex: slot 0),
	// we set justified block root to an empty root.
	justifiedBlockRoot := states[slot-1].JustifiedRoot

	// If an attester has to attest for genesis block.
	if states[slot-1].Slot == params.BeaconConfig().GenesisSlot {
		epochBoundaryRoot = params.BeaconConfig().ZeroHash[:]
		justifiedBlockRoot = params.BeaconConfig().ZeroHash[:]
	}
	attestation.Data.Slot = prevSlot
	attestation.Data.Shard = shard
	attestation.Data.EpochBoundaryRootHash32 = epochBoundaryRoot
	attestation.Data.JustifiedBlockRootHash32 = justifiedBlockRoot
	attestation.Data.JustifiedEpoch = states[slot-1].JustifiedEpoch
	attestation.Data.LatestCrosslink = states[slot-1].LatestCrosslinks[shard]
	attestation.Data.CrosslinkDataRootHash32 = params.BeaconConfig().ZeroHash[:]
	block.Body.Attestations = []*pb.Attestation{attestation}
	return block
}

func advanceChain(t *testing.T, chainService *ChainService, numBlocks int) ([]*pb.BeaconBlock, []*pb.BeaconState) {
	unixTime := time.Unix(0, 0)
	deposits, _ := setupInitialDeposits(t, 8)
	genesisState, err := chainService.initializeBeaconChain(unixTime, deposits, &pb.Eth1Data{})
	if err != nil {
		t.Fatalf("Could not initialize beacon state to disk: %v", err)
	}
	stateRoot, err := hashutil.HashProto(genesisState)
	if err != nil {
		t.Fatal(err)
	}
	genesisBlock := b.NewGenesisBlock(stateRoot[:])

	// Then, we create the chain up to slot 100 in both.
	blocks := make([]*pb.BeaconBlock, numBlocks+1)
	blocks[0] = genesisBlock
	states := make([]*pb.BeaconState, numBlocks+1)
	states[0] = genesisState
	assignments := make(map[uint64]*assignment)
	for idx := range genesisState.ValidatorRegistry {
		committee, shard, slot, _, err :=
			helpers.CommitteeAssignment(genesisState, genesisBlock.Slot, uint64(idx), false)
		if err != nil {
			t.Fatal(err)
		}
		assignments[slot] = &assignment{
			shard,
			uint64(idx),
			committee,
		}
	}
	for i := uint64(1); i <= uint64(numBlocks); i++ {
		block := createBlock(t, i, blocks, states, assignments)
		beaconState, err := chainService.ApplyBlockStateTransition(context.Background(), block, states[i-1])
		if err != nil {
			t.Fatal(err)
		}
		if err := chainService.beaconDB.SaveBlock(block); err != nil {
			t.Fatal(err)
		}
		if err := chainService.beaconDB.UpdateChainHead(context.Background(), block, beaconState); err != nil {
			t.Fatal(err)
		}
		blocks[i] = block
		states[i] = beaconState
	}
	return blocks, states
}

// This function tests the following: when two nodes A and B are running at slot 10
// and node A reorgs back to slot 7 (an epoch boundary), while node B remains the same,
// once the nodes catch up a few blocks later, we expect their state and validator
// balances to remain the same. That is, we expect no deviation in validator balances.
func TestEpochReorg_MatchingStates(t *testing.T) {
	params.UseDemoBeaconConfig()
	// First we setup two independent db's for node A and B.
	beaconDB1 := internal.SetupDB(t)
	beaconDB2 := internal.SetupDB(t)
	defer internal.TeardownDB(t, beaconDB1)
	defer internal.TeardownDB(t, beaconDB2)

	chainService1 := setupBeaconChain(t, beaconDB1, nil)
	chainService2 := setupBeaconChain(t, beaconDB2, nil)
	_, states1 := advanceChain(t, chainService1, 34)
	_, states2 := advanceChain(t, chainService2, 34)

	lastState1 := states1[len(states1)-1]
	lastState2 := states2[len(states2)-1]

	// We expect both nodes to have matching balances after generating N
	// blocks, including attestations, and applying N state transitions.
	balancesNodeA := lastState1.ValidatorBalances
	balancesNodeB := lastState2.ValidatorBalances
	for i := range balancesNodeA {
		if balancesNodeA[i] != balancesNodeB[i] {
			t.Errorf(
				"Expected balance to match at index %d, received %v = %v",
				i,
				balancesNodeA[i],
				balancesNodeB[i],
			)
		}
	}
	t.Logf("Validator balances node A: %v", balancesNodeA)
	t.Logf("Finalized epoch node A: %v", lastState1.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	t.Logf("Justified epoch node A: %v", lastState1.JustifiedEpoch-params.BeaconConfig().GenesisEpoch)
	t.Logf("Validator balances node B: %v", balancesNodeB)
	t.Logf("Finalized epoch node B: %v", lastState2.FinalizedEpoch-params.BeaconConfig().GenesisEpoch)
	t.Logf("Justified epoch node B: %v", lastState2.JustifiedEpoch-params.BeaconConfig().GenesisEpoch)

	// We update attestation targets for node A such that validators point to the block
	// at slot 7 as canonical - then, a reorg to that slot will occur.

	// We then proceed in both nodes normally through several blocks.

	// At this point, once the two nodes are fully caught up, we expect their state,
	// in particular their balances, to be equal.
}