// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/prysmaticlabs/prysm/proto/beacon/rpc/v1 (interfaces: ValidatorServiceServer)

package internal

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1 "github.com/prysmaticlabs/prysm/proto/beacon/rpc/v1"
)

// MockValidatorServiceServer is a mock of ValidatorServiceServer interface
type MockValidatorServiceServer struct {
	ctrl     *gomock.Controller
	recorder *MockValidatorServiceServerMockRecorder
}

// MockValidatorServiceServerMockRecorder is the mock recorder for MockValidatorServiceServer
type MockValidatorServiceServerMockRecorder struct {
	mock *MockValidatorServiceServer
}

// NewMockValidatorServiceServer creates a new mock instance
func NewMockValidatorServiceServer(ctrl *gomock.Controller) *MockValidatorServiceServer {
	mock := &MockValidatorServiceServer{ctrl: ctrl}
	mock.recorder = &MockValidatorServiceServerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockValidatorServiceServer) EXPECT() *MockValidatorServiceServerMockRecorder {
	return m.recorder
}

// CommitteeAssignment mocks base method
func (m *MockValidatorServiceServer) CommitteeAssignment(arg0 context.Context, arg1 *v1.CommitteeAssignmentsRequest) (*v1.CommitteeAssignmentResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CommitteeAssignment", arg0, arg1)
	ret0, _ := ret[0].(*v1.CommitteeAssignmentResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CommitteeAssignment indicates an expected call of CommitteeAssignment
func (mr *MockValidatorServiceServerMockRecorder) CommitteeAssignment(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CommitteeAssignment", reflect.TypeOf((*MockValidatorServiceServer)(nil).CommitteeAssignment), arg0, arg1)
}

// ValidatorIndex mocks base method
func (m *MockValidatorServiceServer) ValidatorIndex(arg0 context.Context, arg1 *v1.ValidatorIndexRequest) (*v1.ValidatorIndexResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidatorIndex", arg0, arg1)
	ret0, _ := ret[0].(*v1.ValidatorIndexResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ValidatorIndex indicates an expected call of ValidatorIndex
func (mr *MockValidatorServiceServerMockRecorder) ValidatorIndex(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidatorIndex", reflect.TypeOf((*MockValidatorServiceServer)(nil).ValidatorIndex), arg0, arg1)
}

// ValidatorPerformance mocks base method
func (m *MockValidatorServiceServer) ValidatorPerformance(arg0 context.Context, arg1 *v1.ValidatorPerformanceRequest) (*v1.ValidatorPerformanceResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidatorPerformance", arg0, arg1)
	ret0, _ := ret[0].(*v1.ValidatorPerformanceResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ValidatorPerformance indicates an expected call of ValidatorPerformance
func (mr *MockValidatorServiceServerMockRecorder) ValidatorPerformance(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidatorPerformance", reflect.TypeOf((*MockValidatorServiceServer)(nil).ValidatorPerformance), arg0, arg1)
}

// ValidatorStatus mocks base method
func (m *MockValidatorServiceServer) ValidatorStatus(arg0 context.Context, arg1 *v1.ValidatorIndexRequest) (*v1.ValidatorStatusResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidatorStatus", arg0, arg1)
	ret0, _ := ret[0].(*v1.ValidatorStatusResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ValidatorStatus indicates an expected call of ValidatorStatus
func (mr *MockValidatorServiceServerMockRecorder) ValidatorStatus(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidatorStatus", reflect.TypeOf((*MockValidatorServiceServer)(nil).ValidatorStatus), arg0, arg1)
}
