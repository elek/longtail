# Storj longtail

This application uploads data to the Storj ecosystem and tries to download induvidual pieces from different servers to measure latency.

Usage:

1. Create test data

```
for i in `seq 100 900`; do echo $i; /tmp/uplink cp file1 sj://base/file$i; done
```

Note: by default only the fastest 80 node is used from a random selected 110 for each upload. To get number even about the slower nodes we need a modified uplink:

```
diff --git private/ecclient/client.go private/ecclient/client.go
index 57c61f24..7dcaef30 100644
--- private/ecclient/client.go
+++ private/ecclient/client.go
@@ -160,10 +160,6 @@ func (ec *ecClient) put(ctx context.Context, limits []*pb.AddressedOrderLimit, p
                successfulHashes[info.i] = info.hash
 
                successfulCount++
-               if int(successfulCount) >= rs.OptimalThreshold() {
-                       // cancelling remaining uploads
-                       piecesCancel()
-               }
        }
 
        defer func() {
```

2. Collect data about download speed:

```
longtail download
```
