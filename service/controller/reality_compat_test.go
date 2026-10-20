package controller_test

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/wyx2685/XrayR/api"
	"github.com/wyx2685/XrayR/common/mylego"
	"github.com/wyx2685/XrayR/service/controller"
	"github.com/xtls/xray-core/app/proxyman"
	vlessinbound "github.com/xtls/xray-core/proxy/vless/inbound"
	"github.com/xtls/xray-core/transport/internet/reality"
)

// Keep the existing panel configuration compatible when updating the core's
// REALITY implementation. Building a config does not contact the target site.
func TestBuildRemoteVlessReality(t *testing.T) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeType := range []string{"V2ray", "Vless"} {
		t.Run(nodeType, func(t *testing.T) {
			node := &api.NodeInfo{
				NodeType:          nodeType,
				Port:              443,
				EnableVless:       true,
				VlessFlow:         "xtls-rprx-vision",
				TransportProtocol: "tcp",
				EnableTLS:         true,
				EnableREALITY:     true,
				REALITYConfig: &api.REALITYConfig{
					Dest:        "www.example.com:443",
					ServerNames: []string{"www.example.com"},
					PrivateKey:  base64.RawURLEncoding.EncodeToString(key.Bytes()),
					ShortIds:    []string{"01234567"},
				},
			}
			cfg, err := controller.InboundBuilder(&controller.Config{
				DisableLocalREALITYConfig: true,
				REALITYConfigs:            &controller.REALITYConfig{},
				CertConfig:                &mylego.CertConfig{CertMode: "none"},
			}, node, "reality-compat")
			if err != nil {
				t.Fatal(err)
			}
			proxy, err := cfg.ProxySettings.GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			if vless, ok := proxy.(*vlessinbound.Config); !ok || vless.Decryption != "none" {
				t.Fatalf("expected the existing VLESS protocol, got %T", proxy)
			}
			receiver, err := cfg.ReceiverSettings.GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			stream := receiver.(*proxyman.ReceiverConfig).StreamSettings
			if stream.ProtocolName != "tcp" || len(stream.SecuritySettings) != 1 {
				t.Fatal("TCP/REALITY transport was not preserved")
			}
			security, err := stream.SecuritySettings[0].GetInstance()
			if err != nil {
				t.Fatal(err)
			}
			r, ok := security.(*reality.Config)
			if !ok {
				t.Fatalf("expected REALITY, got %T", security)
			}
			if r.Dest != node.REALITYConfig.Dest ||
				len(r.ServerNames) != 1 || r.ServerNames[0] != "www.example.com" ||
				!bytes.Equal(r.PrivateKey, key.Bytes()) ||
				len(r.ShortIds) != 1 || !bytes.Equal(r.ShortIds[0], []byte{1, 35, 69, 103, 0, 0, 0, 0}) {
				t.Fatal("existing REALITY target or authentication parameters changed")
			}
		})
	}
}
