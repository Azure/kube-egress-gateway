package nicservice

import (
	"context"

	cniprotocol "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type NicService struct {
	cniprotocol.UnimplementedNicServiceServer
}

func (s *NicService) NicAdd(ctx context.Context, in *cniprotocol.NicAddRequest) (*cniprotocol.NicAddResponse, error) {
	key, _ := wgtypes.GeneratePrivateKey()
	return &cniprotocol.NicAddResponse{
		EndpointIp: "192.168.0.2",
		ListenPort: 54453,
		PublicKey:  key.PublicKey().String(),
	}, nil
}

func (s *NicService) NicDel(ctx context.Context, in *cniprotocol.NicDelRequest) (*cniprotocol.NicDelResponse, error) {
	return &cniprotocol.NicDelResponse{}, nil
}
