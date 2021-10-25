package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/zeebo/errs"
	"io"
	"math/rand"
	"os"
	"storj.io/common/encryption"
	"storj.io/common/grant"
	"storj.io/common/identity"
	"storj.io/common/paths"
	"storj.io/common/pb"
	"storj.io/common/peertls/tlsopts"
	"storj.io/common/rpc"
	"storj.io/common/storj"
	"storj.io/uplink/private/metaclient"
	"storj.io/uplink/private/piecestore"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
	RootCmd.AddCommand(&cobra.Command{
		Use: "download",
		RunE: func(cmd *cobra.Command, args []string) error {
			return downloadtest(context.Background())
		},
	})
}

func downloadtest(ctx context.Context) error {

	db, err := sql.Open("sqlite3", "./download-times.db?_sync=0&_journal_mode=OFF")
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE downloads 
   (key TEXT NOT NULL, 
   address TEXT NOT NULL,
   time INTEGER, 
   size INTEGER,
   duration_ms INTEGER, 
   destination TEXT,
   PRIMARY KEY(key,address,time))
`)
	if err != nil {
		log.Warn().Msg(err.Error())
	}

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

	for {
		i := rand.Intn(675) + 100
		key := fmt.Sprintf("file%d", i)
		encPath, err := encryption.EncryptPathWithStoreCipher(bucketName, paths.NewUnencrypted(key), inner.EncAccess.Store)
		if err != nil {
			return errs.Wrap(err)
		}
		res, err := metainfoClient.DownloadObject(ctx, metaclient.DownloadObjectParams{
			Bucket:             []byte(bucketName),
			EncryptedObjectKey: []byte(encPath.Raw()),
			Range: metaclient.StreamRange{
				Mode:  metaclient.StreamRangeAll,
				Start: 0,
			},
		})
		for _, k := range res.DownloadedSegments {
			log.Info().Msgf("Checking %d pieces of key %s", len(k.Limits), key)
			for _, x := range k.Limits {
				if x != nil {
					address := x.GetStorageNodeAddress().Address
					host := strings.Split(address, ":")[0]
					lastData, err := checkLastData(db, host)
					if err != nil {
						return err
					}
					lastCheck := time.Now().Sub(lastData)
					if lastCheck > time.Hour*24 {

						nodeUrl := storj.NodeURL{
							ID:      x.GetLimit().StorageNodeId,
							Address: address,
						}
						contextWithTimeout, _ := context.WithTimeout(ctx, time.Second*30)
						size, duration, err := measureDownload(contextWithTimeout, dialer, nodeUrl, x.GetLimit(), k.Info.PiecePrivateKey)
						if err != nil {
							log.Warn().Msgf("Error on downloading piece from %s %v", address, err)
						}

						err = updateDb(db, key, size, duration, host)
						if err != nil {
							return err
						}
					} else {
						log.Info().Msgf("Ignoring host %s as it's checked %f minutes ago", host, lastCheck.Minutes())
					}
				}

			}
		}
	}
	fmt.Println(len(servers))

	return nil

}

func updateDb(db *sql.DB, key string, size int64, duration time.Duration, address string) error {
	_, err := db.Exec("INSERT INTO downloads (key,address,time,size,duration_ms,destination) VALUES (?,?,?,?,?,?)", key, address, time.Now().Unix(), size, duration.Milliseconds(), os.Getenv("LONGTAIL_HOST"))
	return err
}

func checkLastData(db *sql.DB, address string) (time.Time, error) {
	rows, err := db.Query("select time from downloads WHERE address = ? order by time desc limit 1", address)
	if err != nil {
		return time.Now(), err
	}
	defer rows.Close()
	if !rows.Next() {
		return time.Unix(0, 0), nil
	} else {
		var timestamp int64
		err = rows.Scan(&timestamp)
		if err != nil {
			return time.Now(), err
		}
		return time.Unix(timestamp, 0), nil
	}
}

func measureDownload(ctx context.Context, dialer rpc.Dialer, nodeUrl storj.NodeURL, orderLimit *pb.OrderLimit, key storj.PiecePrivateKey) (size int64, duration time.Duration, err error) {
	log.Info().Msgf("Downloading piece from from %s", nodeUrl.Address)

	storagenodeClient, err := piecestore.Dial(ctx, dialer, nodeUrl, piecestore.DefaultConfig)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	downloader, err := storagenodeClient.Download(ctx, orderLimit, key, 0, orderLimit.Limit)
	if err != nil {
		return 0, 0, err
	}
	buffer := bytes.NewBuffer([]byte{})
	downloadedBytes, err := io.Copy(buffer, downloader)
	if err != nil {
		return 0, 0, err
	}
	duration = time.Now().Sub(start)
	log.Info().Msgf("Downloaded %d bytes from %s under %d milliseconds", downloadedBytes, nodeUrl.Address, duration.Milliseconds())

	err = storagenodeClient.Close()
	if err != nil {
		return 0, 0, err
	}
	return downloadedBytes, duration, nil
}
