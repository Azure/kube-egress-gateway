package to

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
)

func Ptr[T any](v T) *T {
	return to.Ptr(v)
}

func Val[T any](v *T) T {
	var empty T
	if v == nil {
		return empty
	}
	return *v
}
