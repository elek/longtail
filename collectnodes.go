package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/zeebo/errs"
	"os"
	"storj.io/common/encryption"
	"storj.io/common/grant"
	"storj.io/common/identity"
	"storj.io/common/paths"
	"storj.io/common/peertls/tlsopts"
	"storj.io/common/rpc"
	"storj.io/common/storj"
	"storj.io/uplink/private/metaclient"
)

func init() {

	RootCmd.AddCommand(&cobra.Command{
		Use: "collect",
		RunE: func(cmd *cobra.Command, args []string) error {
			return CollectIPs(context.Background())
		},
	})
}

func CollectIPs(ctx context.Context) error {
	ident, err := identity.NewFullIdentity(ctx, identity.NewCAOptions{
		Difficulty:  0,
		Concurrency: 1,
	})
	if err != nil {
		return err
	}

	tlsConfig := tlsopts.Config{
		UsePeerCAWhitelist: false,
		PeerIDVersions:     "0",
	}

	tlsOptions, err := tlsopts.NewOptions(ident, tlsConfig, nil)
	if err != nil {
		return err
	}

	dialer := rpc.NewDefaultDialer(tlsOptions)

	inner, err := grant.ParseAccess(os.Getenv("STORJ_ACCESS"))
	if err != nil {
		return err
	}

	nodeURL, err := storj.ParseNodeURL(inner.SatelliteAddress)

	metainfoClient, err := metaclient.DialNodeURL(ctx,
		dialer,
		nodeURL.String(),
		inner.APIKey,
		"My personal client")
	if err != nil {
		return err
	}
	defer metainfoClient.Close()

	bucketName := "base"
	servers := make(map[string]int)
	for i := 1; i < 600; i++ {
		key := fmt.Sprintf("file%d", i)
		encPath, err := encryption.EncryptPathWithStoreCipher(bucketName, paths.NewUnencrypted(key), inner.EncAccess.Store)
		if err != nil {
			return errs.Wrap(err)
			fmt.Println("%v", err)
		}
		resp, err := metainfoClient.GetObjectIPs(ctx, metaclient.GetObjectIPsParams{
			Bucket:        []byte(bucketName),
			EncryptedPath: []byte(encPath.Raw()),
		})
		if err != nil {
			continue
			//return errs.New("Couldn't get key %s (%v)", key, err)
		}
		fmt.Printf("Collection IPS for file %s with pieces %d\n", key, len(resp.IPPorts))
		for _, info := range resp.IPPorts {
			hostPort := string(info)
			if _, found := servers[hostPort]; !found {
				servers[hostPort] = 0
			}
			servers[hostPort] = servers[hostPort] + 1
		}
	}
	fmt.Println(len(servers))

	return nil

}
