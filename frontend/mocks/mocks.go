// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/moratsam/etherscan/frontend (interfaces: ScoreStoreAPI)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	scorestore "github.com/moratsam/etherscan/scorestore"
)

// MockScoreStoreAPI is a mock of ScoreStoreAPI interface.
type MockScoreStoreAPI struct {
	ctrl     *gomock.Controller
	recorder *MockScoreStoreAPIMockRecorder
}

// MockScoreStoreAPIMockRecorder is the mock recorder for MockScoreStoreAPI.
type MockScoreStoreAPIMockRecorder struct {
	mock *MockScoreStoreAPI
}

// NewMockScoreStoreAPI creates a new mock instance.
func NewMockScoreStoreAPI(ctrl *gomock.Controller) *MockScoreStoreAPI {
	mock := &MockScoreStoreAPI{ctrl: ctrl}
	mock.recorder = &MockScoreStoreAPIMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockScoreStoreAPI) EXPECT() *MockScoreStoreAPIMockRecorder {
	return m.recorder
}

// Search mocks base method.
func (m *MockScoreStoreAPI) Search(arg0 scorestore.Query) (scorestore.ScoreIterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Search", arg0)
	ret0, _ := ret[0].(scorestore.ScoreIterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Search indicates an expected call of Search.
func (mr *MockScoreStoreAPIMockRecorder) Search(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Search", reflect.TypeOf((*MockScoreStoreAPI)(nil).Search), arg0)
}
