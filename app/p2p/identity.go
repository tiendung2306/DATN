package p2p

import (
	"fmt"
	"log/slog"

	"app/db"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const Libp2pPrivKeyConfigKey = "libp2p_priv_key"

// GetOrCreateIdentity retrieves the persistent Libp2p private key from the
// database or generates a new one on first run.
func GetOrCreateIdentity(database *db.Database) (crypto.PrivKey, error) {
	privKeyBytes, err := database.GetConfig(Libp2pPrivKeyConfigKey)
	if err != nil && !db.IsNotFound(err) {
		return nil, fmt.Errorf("failed to query Libp2p identity: %w", err)
	}

	if err == nil {
		privKey, err := crypto.UnmarshalPrivateKey(privKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal Libp2p private key: %w", err)
		}
		peerID, _ := peer.IDFromPrivateKey(privKey)
		slog.Info("Loaded existing Libp2p identity", "peerID", peerID.String())
		return privKey, nil
	}

	// No key found — generate a new Ed25519 key pair.
	slog.Info("Generating new Libp2p identity...")
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Libp2p key pair: %w", err)
	}

	rawPriv, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Libp2p private key: %w", err)
	}

	if err := database.SetConfig(Libp2pPrivKeyConfigKey, rawPriv); err != nil {
		return nil, fmt.Errorf("failed to persist Libp2p private key: %w", err)
	}

	peerID, _ := peer.IDFromPrivateKey(priv)
	slog.Info("Created and saved new Libp2p identity", "peerID", peerID.String())
	return priv, nil
}
