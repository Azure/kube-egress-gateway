package utils

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

func PollUntilDone[ResponseType interface{}](ctx context.Context, asyncHandler func() (*runtime.Poller[ResponseType], error)) (*ResponseType, error) {
	pollerResp, err := asyncHandler()
	if err != nil {
		return nil, err
	}

	resp, err := pollerResp.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
