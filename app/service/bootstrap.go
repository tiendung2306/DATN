package service

import (
	"fmt"

	"app/adapter/p2p"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2pPeer "github.com/libp2p/go-libp2p/core/peer"
)

// BuildAdminBootstrapAddr constructs the full multiaddr for Admin's P2P endpoint.
// The /p2p/PEERID suffix is mandatory for Noise authentication.
func BuildAdminBootstrapAddr(privKey p2pCrypto.PrivKey, port int) (string, error) {
	peerID, err := p2pPeer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("derive admin PeerID: %w", err)
	}
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", p2p.GetBestLocalIP(), port, peerID), nil
}
