package service

import "google.golang.org/protobuf/runtime/protoimpl"

type Status struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Version             string   `protobuf:"bytes,1,opt,name=Version,proto3" json:"Version,omitempty"`
	Hostname            string   `protobuf:"bytes,2,opt,name=Hostname,proto3" json:"Hostname,omitempty"`
	Domain              string   `protobuf:"bytes,3,opt,name=Domain,proto3" json:"Domain,omitempty"`
	PublicIP            string   `protobuf:"bytes,4,opt,name=PublicIP,proto3" json:"PublicIP,omitempty"`
	GRPCPort            string   `protobuf:"bytes,5,opt,name=gRPCPort,proto3" json:"gRPCPort,omitempty"`
	PrivateIP           string   `protobuf:"bytes,6,opt,name=PrivateIP,proto3" json:"PrivateIP,omitempty"`
	HttpPort            string   `protobuf:"bytes,7,opt,name=HttpPort,proto3" json:"HttpPort,omitempty"`
	Region              string   `protobuf:"bytes,8,opt,name=Region,proto3" json:"Region,omitempty"`
	VPNPort             string   `protobuf:"bytes,9,opt,name=VPNPort,proto3" json:"VPNPort,omitempty"`
	PublicKey           string   `protobuf:"bytes,10,opt,name=PublicKey,proto3" json:"PublicKey,omitempty"`
	PersistentKeepalive int64    `protobuf:"varint,11,opt,name=PersistentKeepalive,proto3" json:"PersistentKeepalive,omitempty"`
	DNS                 []string `protobuf:"bytes,12,rep,name=DNS,proto3" json:"DNS,omitempty"`
}
