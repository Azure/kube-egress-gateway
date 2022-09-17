package nicservice

import (
	"context"

	"github.com/Azure/kube-egress-gateway/pkg/cniprotocol"
)

type NicService struct {
	cniprotocol.UnimplementedNicServiceServer
}

func (s *NicService) NicAdd(ctx context.Context, in *cniprotocol.CNIAddRequest) (*cniprotocol.CNIAddResponse, error) {
	panic("implement me")
}

func (s *NicService) NicDel(ctx context.Context, in *cniprotocol.CNIDeleteRequest) (*cniprotocol.CNIDeleteResponse, error) {
	panic("implement me")
}
