package p2p

import (
	"fmt"
	"log/slog"

	"backend/db"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const Libp2pPrivKeyConfigKey = "libp2p_priv_key"

// GetOrCreateIdentity retrieves the libp2p private key from the database or creates a new one.
func GetOrCreateIdentity(database *db.Database) (crypto.PrivKey, error) {
	var privKeyBytes []byte
	err := database.Conn.QueryRow("SELECT value FROM system_config WHERE key = ?", Libp2pPrivKeyConfigKey).Scan(&privKeyBytes)

	if err == nil {
		// Key exists, unmarshal it
		privKey, err := crypto.UnmarshalPrivateKey(privKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
		}
		
		peerID, _ := peer.IDFromPrivateKey(privKey)
		slog.Info("Loaded existing P2P identity", "peerID", peerID.String())
		return privKey, nil
	}

	// Key doesn't exist, generate a new one
	slog.Info("Generating new P2P identity...")
	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	rawPriv, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	_, err = database.Conn.Exec("INSERT INTO system_config (key, value) VALUES (?, ?)", Libp2pPrivKeyConfigKey, rawPriv)
	if err != nil {
		return nil, fmt.Errorf("failed to save private key to db: %w", err)
	}

	peerID, _ := peer.IDFromPrivateKey(priv)
	slog.Info("Created and saved new P2P identity", "peerID", peerID.String())
	return priv, nil
}
