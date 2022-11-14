package nicservice

import (
	"context"

	"github.com/Azure/kube-egress-gateway/pkg/cniprotocol"
)

type NicService struct {
	cniprotocol.UnimplementedNicServiceServer
}

func (s *NicService) NicAdd(ctx context.Context, in *cniprotocol.NicAddRequest) (*cniprotocol.NicAddResponse, error) {
	panic("implement me")
}

func (s *NicService) NicDel(ctx context.Context, in *cniprotocol.NicDelRequest) (*cniprotocol.NicDelResponse, error) {
	panic("implement me")
}
