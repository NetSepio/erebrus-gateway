package p2pnode

import (
	"context"

	"github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/host"
	"github.com/TheLazarusNetwork/erebrus-gateway/app/p2p-Node/service"
)

func Init() {
	ctx := context.Background()
	h := host.CreateHost()

	service.Init(h, ctx)

}
