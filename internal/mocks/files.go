// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/D8-X/d8x-cli/internal/files (interfaces: EmbedFileCopier,HostsFileInteractor,FSInteractor)

// Package mocks is a generated GoMock package.
package mocks

import (
	embed "embed"
	fs "io/fs"
	reflect "reflect"

	files "github.com/D8-X/d8x-cli/internal/files"
	gomock "go.uber.org/mock/gomock"
)

// MockEmbedFileCopier is a mock of EmbedFileCopier interface.
type MockEmbedFileCopier struct {
	ctrl     *gomock.Controller
	recorder *MockEmbedFileCopierMockRecorder
}

// MockEmbedFileCopierMockRecorder is the mock recorder for MockEmbedFileCopier.
type MockEmbedFileCopierMockRecorder struct {
	mock *MockEmbedFileCopier
}

// NewMockEmbedFileCopier creates a new mock instance.
func NewMockEmbedFileCopier(ctrl *gomock.Controller) *MockEmbedFileCopier {
	mock := &MockEmbedFileCopier{ctrl: ctrl}
	mock.recorder = &MockEmbedFileCopierMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockEmbedFileCopier) EXPECT() *MockEmbedFileCopierMockRecorder {
	return m.recorder
}

// Copy mocks base method.
func (m *MockEmbedFileCopier) Copy(arg0 embed.FS, arg1 ...files.EmbedCopierOp) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Copy", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// Copy indicates an expected call of Copy.
func (mr *MockEmbedFileCopierMockRecorder) Copy(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Copy", reflect.TypeOf((*MockEmbedFileCopier)(nil).Copy), varargs...)
}

// CopyMultiToDest mocks base method.
func (m *MockEmbedFileCopier) CopyMultiToDest(arg0 embed.FS, arg1 string, arg2 ...string) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CopyMultiToDest", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// CopyMultiToDest indicates an expected call of CopyMultiToDest.
func (mr *MockEmbedFileCopierMockRecorder) CopyMultiToDest(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CopyMultiToDest", reflect.TypeOf((*MockEmbedFileCopier)(nil).CopyMultiToDest), varargs...)
}

// MockHostsFileInteractor is a mock of HostsFileInteractor interface.
type MockHostsFileInteractor struct {
	ctrl     *gomock.Controller
	recorder *MockHostsFileInteractorMockRecorder
}

// MockHostsFileInteractorMockRecorder is the mock recorder for MockHostsFileInteractor.
type MockHostsFileInteractorMockRecorder struct {
	mock *MockHostsFileInteractor
}

// NewMockHostsFileInteractor creates a new mock instance.
func NewMockHostsFileInteractor(ctrl *gomock.Controller) *MockHostsFileInteractor {
	mock := &MockHostsFileInteractor{ctrl: ctrl}
	mock.recorder = &MockHostsFileInteractorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHostsFileInteractor) EXPECT() *MockHostsFileInteractorMockRecorder {
	return m.recorder
}

// GetAllPublicIps mocks base method.
func (m *MockHostsFileInteractor) GetAllPublicIps() []string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAllPublicIps")
	ret0, _ := ret[0].([]string)
	return ret0
}

// GetAllPublicIps indicates an expected call of GetAllPublicIps.
func (mr *MockHostsFileInteractorMockRecorder) GetAllPublicIps() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAllPublicIps", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetAllPublicIps))
}

// GetBrokerPublicIp mocks base method.
func (m *MockHostsFileInteractor) GetBrokerPublicIp() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBrokerPublicIp")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBrokerPublicIp indicates an expected call of GetBrokerPublicIp.
func (mr *MockHostsFileInteractorMockRecorder) GetBrokerPublicIp() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBrokerPublicIp", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetBrokerPublicIp))
}

// GetLines mocks base method.
func (m *MockHostsFileInteractor) GetLines() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLines")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetLines indicates an expected call of GetLines.
func (mr *MockHostsFileInteractorMockRecorder) GetLines() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLines", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetLines))
}

// GetMangerPrivateIp mocks base method.
func (m *MockHostsFileInteractor) GetMangerPrivateIp() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMangerPrivateIp")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMangerPrivateIp indicates an expected call of GetMangerPrivateIp.
func (mr *MockHostsFileInteractorMockRecorder) GetMangerPrivateIp() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMangerPrivateIp", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetMangerPrivateIp))
}

// GetMangerPublicIp mocks base method.
func (m *MockHostsFileInteractor) GetMangerPublicIp() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMangerPublicIp")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMangerPublicIp indicates an expected call of GetMangerPublicIp.
func (mr *MockHostsFileInteractorMockRecorder) GetMangerPublicIp() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMangerPublicIp", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetMangerPublicIp))
}

// GetWorkerIps mocks base method.
func (m *MockHostsFileInteractor) GetWorkerIps() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWorkerIps")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetWorkerIps indicates an expected call of GetWorkerIps.
func (mr *MockHostsFileInteractorMockRecorder) GetWorkerIps() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWorkerIps", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetWorkerIps))
}

// GetWorkerPrivateIps mocks base method.
func (m *MockHostsFileInteractor) GetWorkerPrivateIps() ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWorkerPrivateIps")
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetWorkerPrivateIps indicates an expected call of GetWorkerPrivateIps.
func (mr *MockHostsFileInteractorMockRecorder) GetWorkerPrivateIps() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWorkerPrivateIps", reflect.TypeOf((*MockHostsFileInteractor)(nil).GetWorkerPrivateIps))
}

// WriteLines mocks base method.
func (m *MockHostsFileInteractor) WriteLines(arg0 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteLines", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// WriteLines indicates an expected call of WriteLines.
func (mr *MockHostsFileInteractorMockRecorder) WriteLines(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WriteLines", reflect.TypeOf((*MockHostsFileInteractor)(nil).WriteLines), arg0)
}

// MockFSInteractor is a mock of FSInteractor interface.
type MockFSInteractor struct {
	ctrl     *gomock.Controller
	recorder *MockFSInteractorMockRecorder
}

// MockFSInteractorMockRecorder is the mock recorder for MockFSInteractor.
type MockFSInteractorMockRecorder struct {
	mock *MockFSInteractor
}

// NewMockFSInteractor creates a new mock instance.
func NewMockFSInteractor(ctrl *gomock.Controller) *MockFSInteractor {
	mock := &MockFSInteractor{ctrl: ctrl}
	mock.recorder = &MockFSInteractorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFSInteractor) EXPECT() *MockFSInteractorMockRecorder {
	return m.recorder
}

// ReplaceAndCopy mocks base method.
func (m *MockFSInteractor) ReplaceAndCopy(arg0, arg1 string, arg2 []files.ReplacementTuple) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReplaceAndCopy", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReplaceAndCopy indicates an expected call of ReplaceAndCopy.
func (mr *MockFSInteractorMockRecorder) ReplaceAndCopy(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReplaceAndCopy", reflect.TypeOf((*MockFSInteractor)(nil).ReplaceAndCopy), arg0, arg1, arg2)
}

// Stat mocks base method.
func (m *MockFSInteractor) Stat(arg0 string) (fs.FileInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat", arg0)
	ret0, _ := ret[0].(fs.FileInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Stat indicates an expected call of Stat.
func (mr *MockFSInteractorMockRecorder) Stat(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockFSInteractor)(nil).Stat), arg0)
}

// WriteFile mocks base method.
func (m *MockFSInteractor) WriteFile(arg0 string, arg1 []byte) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteFile", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// WriteFile indicates an expected call of WriteFile.
func (mr *MockFSInteractorMockRecorder) WriteFile(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WriteFile", reflect.TypeOf((*MockFSInteractor)(nil).WriteFile), arg0, arg1)
}
