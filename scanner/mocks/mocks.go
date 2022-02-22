// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/moratsam/etherscan/scanner (interfaces: ETHClient,Graph)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	types "github.com/ethereum/go-ethereum/core/types"
	gomock "github.com/golang/mock/gomock"
	graph "github.com/moratsam/etherscan/txgraph/graph"
)

// MockETHClient is a mock of ETHClient interface.
type MockETHClient struct {
	ctrl     *gomock.Controller
	recorder *MockETHClientMockRecorder
}

// MockETHClientMockRecorder is the mock recorder for MockETHClient.
type MockETHClientMockRecorder struct {
	mock *MockETHClient
}

// NewMockETHClient creates a new mock instance.
func NewMockETHClient(ctrl *gomock.Controller) *MockETHClient {
	mock := &MockETHClient{ctrl: ctrl}
	mock.recorder = &MockETHClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockETHClient) EXPECT() *MockETHClientMockRecorder {
	return m.recorder
}

// BlockByNumber mocks base method.
func (m *MockETHClient) BlockByNumber(arg0 int) (*types.Block, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BlockByNumber", arg0)
	ret0, _ := ret[0].(*types.Block)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BlockByNumber indicates an expected call of BlockByNumber.
func (mr *MockETHClientMockRecorder) BlockByNumber(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BlockByNumber", reflect.TypeOf((*MockETHClient)(nil).BlockByNumber), arg0)
}

// MockGraph is a mock of Graph interface.
type MockGraph struct {
	ctrl     *gomock.Controller
	recorder *MockGraphMockRecorder
}

// MockGraphMockRecorder is the mock recorder for MockGraph.
type MockGraphMockRecorder struct {
	mock *MockGraph
}

// NewMockGraph creates a new mock instance.
func NewMockGraph(ctrl *gomock.Controller) *MockGraph {
	mock := &MockGraph{ctrl: ctrl}
	mock.recorder = &MockGraphMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGraph) EXPECT() *MockGraphMockRecorder {
	return m.recorder
}

// InsertTxs mocks base method.
func (m *MockGraph) InsertTxs(arg0 []*graph.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertTxs", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertTxs indicates an expected call of InsertTxs.
func (mr *MockGraphMockRecorder) InsertTxs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertTxs", reflect.TypeOf((*MockGraph)(nil).InsertTxs), arg0)
}

// UpsertBlock mocks base method.
func (m *MockGraph) UpsertBlock(arg0 *graph.Block) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertBlock", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertBlock indicates an expected call of UpsertBlock.
func (mr *MockGraphMockRecorder) UpsertBlock(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertBlock", reflect.TypeOf((*MockGraph)(nil).UpsertBlock), arg0)
}

// UpsertWallet mocks base method.
func (m *MockGraph) UpsertWallet(arg0 *graph.Wallet) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertWallet", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertWallet indicates an expected call of UpsertWallet.
func (mr *MockGraphMockRecorder) UpsertWallet(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertWallet", reflect.TypeOf((*MockGraph)(nil).UpsertWallet), arg0)
}
