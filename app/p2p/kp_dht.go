package p2p

import (
	"context"
	"fmt"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
)

// DHT key scheme (all under /app/ namespace, which uses appDHTValidator):
//
//	/app/kp/{peerID}           — public KeyPackage bytes for peerID
//	/app/welcome/{peerID}/{groupID} — Welcome bytes from creator to invitee
//
// Both are plain binary blobs. Content-level integrity relies on MLS signatures.

func kpDHTKey(id peer.ID) string {
	return "/app/kp/" + id.String()
}

func welcomeDHTKey(inviteePeerID peer.ID, groupID string) string {
	return "/app/welcome/" + inviteePeerID.String() + "/" + groupID
}

// AdvertiseKeyPackage puts the public KeyPackage bytes into the DHT so that any
// peer can fetch them to perform AddMembers — even while the holder is offline.
func AdvertiseKeyPackage(ctx context.Context, d *dht.IpfsDHT, id peer.ID, kpBytes []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := d.PutValue(ctx, kpDHTKey(id), kpBytes); err != nil {
		return fmt.Errorf("DHT AdvertiseKeyPackage: %w", err)
	}
	return nil
}

// FetchKeyPackage retrieves the public KeyPackage bytes for targetID from the DHT.
// Returns an error if the peer has not yet advertised a KP.
func FetchKeyPackage(ctx context.Context, d *dht.IpfsDHT, targetID peer.ID) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	kp, err := d.GetValue(ctx, kpDHTKey(targetID))
	if err != nil {
		return nil, fmt.Errorf("DHT FetchKeyPackage(%s): %w", targetID, err)
	}
	return kp, nil
}

// StoreWelcomeInDHT publishes Welcome bytes to the DHT so the invitee can
// retrieve them even if they come online after the creator.  Used as a
// secondary delivery channel alongside the direct stream approach.
func StoreWelcomeInDHT(ctx context.Context, d *dht.IpfsDHT, inviteePeerID peer.ID, groupID string, welcomeBytes []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := d.PutValue(ctx, welcomeDHTKey(inviteePeerID, groupID), welcomeBytes); err != nil {
		return fmt.Errorf("DHT StoreWelcome: %w", err)
	}
	return nil
}

// FetchWelcomeFromDHT retrieves Welcome bytes for (myPeerID, groupID).
func FetchWelcomeFromDHT(ctx context.Context, d *dht.IpfsDHT, myPeerID peer.ID, groupID string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	wb, err := d.GetValue(ctx, welcomeDHTKey(myPeerID, groupID))
	if err != nil {
		return nil, fmt.Errorf("DHT FetchWelcome(%s/%s): %w", myPeerID, groupID, err)
	}
	return wb, nil
}
