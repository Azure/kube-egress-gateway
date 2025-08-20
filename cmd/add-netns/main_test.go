package main

import (
	"fmt"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
)

func TestEnsureNS_ExistingNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockNetns := mocknetnswrapper.NewMockInterface(ctrl)
	mockNetns.EXPECT().GetNS("test-ns").Return(&mocknetnswrapper.MockNetNS{Name: "test-ns"}, nil)

	err := ensureNS(mockNetns, "test-ns")
	assert.NoError(t, err)
}

func TestEnsureNS_UnknownError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockNetns := mocknetnswrapper.NewMockInterface(ctrl)
	mockNetns.EXPECT().GetNS("test-ns").Return(nil, fmt.Errorf("unknown get error"))

	err := ensureNS(mockNetns, "test-ns")
	assert.ErrorContains(t, err, "failed to get network namespace")
}

func TestEnsureNS_CreateNS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockNetns := mocknetnswrapper.NewMockInterface(ctrl)
	mockNetns.EXPECT().GetNS("test-ns").Return(nil, ns.NSPathNotExistErr{})
	mockNetns.EXPECT().NewNS("test-ns").Return(&mocknetnswrapper.MockNetNS{Name: "test-ns"}, nil)

	err := ensureNS(mockNetns, "test-ns")
	assert.NoError(t, err)
}
