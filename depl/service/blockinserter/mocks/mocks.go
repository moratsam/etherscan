// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/moratsam/etherscan/depl/service/scanner (interfaces: ETHClient,GraphAPI)

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

// MockGraphAPI is a mock of GraphAPI interface.
type MockGraphAPI struct {
	ctrl     *gomock.Controller
	recorder *MockGraphAPIMockRecorder
}

// MockGraphAPIMockRecorder is the mock recorder for MockGraphAPI.
type MockGraphAPIMockRecorder struct {
	mock *MockGraphAPI
}

// NewMockGraphAPI creates a new mock instance.
func NewMockGraphAPI(ctrl *gomock.Controller) *MockGraphAPI {
	mock := &MockGraphAPI{ctrl: ctrl}
	mock.recorder = &MockGraphAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGraphAPI) EXPECT() *MockGraphAPIMockRecorder {
	return m.recorder
}

// Blocks mocks base method.
func (m *MockGraphAPI) Blocks() (graph.BlockIterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Blocks")
	ret0, _ := ret[0].(graph.BlockIterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Blocks indicates an expected call of Blocks.
func (mr *MockGraphAPIMockRecorder) Blocks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Blocks", reflect.TypeOf((*MockGraphAPI)(nil).Blocks))
}

// InsertTxs mocks base method.
func (m *MockGraphAPI) InsertTxs(arg0 []*graph.Tx) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertTxs", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// InsertTxs indicates an expected call of InsertTxs.
func (mr *MockGraphAPIMockRecorder) InsertTxs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertTxs", reflect.TypeOf((*MockGraphAPI)(nil).InsertTxs), arg0)
}

// UpsertBlock mocks base method.
func (m *MockGraphAPI) UpsertBlock(arg0 *graph.Block) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertBlock", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertBlock indicates an expected call of UpsertBlock.
func (mr *MockGraphAPIMockRecorder) UpsertBlock(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertBlock", reflect.TypeOf((*MockGraphAPI)(nil).UpsertBlock), arg0)
}

// UpsertWallets mocks base method.
func (m *MockGraphAPI) UpsertWallets(arg0 []*graph.Wallet) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertWallets", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// UpsertWallets indicates an expected call of UpsertWallets.
func (mr *MockGraphAPIMockRecorder) UpsertWallets(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertWallets", reflect.TypeOf((*MockGraphAPI)(nil).UpsertWallets), arg0)
}
