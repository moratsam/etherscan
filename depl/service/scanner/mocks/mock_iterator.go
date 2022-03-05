// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/moratsam/etherscan/txgraph/graph (interfaces: BlockIterator)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	graph "github.com/moratsam/etherscan/txgraph/graph"
)

// MockBlockIterator is a mock of BlockIterator interface.
type MockBlockIterator struct {
	ctrl     *gomock.Controller
	recorder *MockBlockIteratorMockRecorder
}

// MockBlockIteratorMockRecorder is the mock recorder for MockBlockIterator.
type MockBlockIteratorMockRecorder struct {
	mock *MockBlockIterator
}

// NewMockBlockIterator creates a new mock instance.
func NewMockBlockIterator(ctrl *gomock.Controller) *MockBlockIterator {
	mock := &MockBlockIterator{ctrl: ctrl}
	mock.recorder = &MockBlockIteratorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBlockIterator) EXPECT() *MockBlockIteratorMockRecorder {
	return m.recorder
}

// Block mocks base method.
func (m *MockBlockIterator) Block() *graph.Block {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Block")
	ret0, _ := ret[0].(*graph.Block)
	return ret0
}

// Block indicates an expected call of Block.
func (mr *MockBlockIteratorMockRecorder) Block() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Block", reflect.TypeOf((*MockBlockIterator)(nil).Block))
}

// Close mocks base method.
func (m *MockBlockIterator) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockBlockIteratorMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockBlockIterator)(nil).Close))
}

// Error mocks base method.
func (m *MockBlockIterator) Error() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Error")
	ret0, _ := ret[0].(error)
	return ret0
}

// Error indicates an expected call of Error.
func (mr *MockBlockIteratorMockRecorder) Error() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Error", reflect.TypeOf((*MockBlockIterator)(nil).Error))
}

// Next mocks base method.
func (m *MockBlockIterator) Next() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next")
	ret0, _ := ret[0].(bool)
	return ret0
}

// Next indicates an expected call of Next.
func (mr *MockBlockIteratorMockRecorder) Next() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockBlockIterator)(nil).Next))
}
