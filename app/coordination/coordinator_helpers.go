package coordination

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/libp2p/go-libp2p/core/peer"
)

func summarizeBufferedProposal(p BufferedProposal) CommitAuditProposalSummary {
	return CommitAuditProposalSummary{
		ProposalType: p.Type,
		OperationID:  p.OperationID,
		TargetPeerID: p.TargetPeerID,
		RequestID:    p.RequestID,
		GroupType:    p.GroupType,
		CategoryID:   p.CategoryID,
	}
}

// buildAddDeliveriesFromBatch projects routing metadata for Add proposals in a
// mixed commit batch. The same Welcome bytes are referenced by WelcomeHash
// across deliveries because OpenMLS emits a single combined Welcome per commit.
func buildAddDeliveriesFromBatch(batch []BufferedProposal, welcomeBytes []byte) []AddCommitDelivery {
	if len(batch) == 0 {
		return nil
	}
	var welcomeHash []byte
	if len(welcomeBytes) > 0 {
		sum := sha256.Sum256(welcomeBytes)
		welcomeHash = sum[:]
	}
	out := make([]AddCommitDelivery, 0, len(batch))
	for _, p := range batch {
		if p.Type != ProposalAdd {
			continue
		}
		out = append(out, AddCommitDelivery{
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
			WelcomeHash:    welcomeHash,
		})
	}
	return out
}

func proposalMsgsFromBatch(batch []BufferedProposal) []ProposalMsg {
	if len(batch) == 0 {
		return nil
	}
	out := make([]ProposalMsg, 0, len(batch))
	for _, p := range batch {
		out = append(out, ProposalMsg{
			ProposalType:   p.Type,
			Data:           append([]byte(nil), p.Data...),
			ProposalRef:    append([]byte(nil), p.ProposalRef...),
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
		})
	}
	return out
}

func bufferedBatchFromProposalMsgs(proposals []ProposalMsg) []BufferedProposal {
	if len(proposals) == 0 {
		return nil
	}
	out := make([]BufferedProposal, 0, len(proposals))
	for _, p := range proposals {
		out = append(out, BufferedProposal{
			Type:           p.ProposalType,
			Data:           append([]byte(nil), p.Data...),
			ProposalRef:    append([]byte(nil), p.ProposalRef...),
			OperationID:    p.OperationID,
			TargetPeerID:   p.TargetPeerID,
			RequestID:      p.RequestID,
			GroupType:      p.GroupType,
			CategoryID:     p.CategoryID,
			KeyPackageHash: append([]byte(nil), p.KeyPackageHash...),
		})
	}
	return out
}

func proposalBytesFromMsgs(proposals []ProposalMsg) [][]byte {
	if len(proposals) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(proposals))
	for _, p := range proposals {
		out = append(out, append([]byte(nil), p.Data...))
	}
	return out
}

func cloneBytesList(in [][]byte) [][]byte {
	if len(in) == 0 {
		return nil
	}
	out := make([][]byte, len(in))
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

func proposalRefSetsEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	aa := cloneBytesList(a)
	bb := cloneBytesList(b)
	sortBytesList(aa)
	sortBytesList(bb)
	for i := range aa {
		if !bytes.Equal(aa[i], bb[i]) {
			return false
		}
	}
	return true
}

func sortBytesList(items [][]byte) {
	sort.SliceStable(items, func(i, j int) bool {
		return bytes.Compare(items[i], items[j]) < 0
	})
}

func removesPeer(batch []BufferedProposal, pid peer.ID) bool {
	if pid == "" {
		return false
	}
	for _, p := range batch {
		if p.Type == ProposalRemove && p.TargetPeerID == pid.String() {
			return true
		}
	}
	return false
}

func batchContainsType(batch []BufferedProposal, pType ProposalType) bool {
	for _, p := range batch {
		if p.Type == pType {
			return true
		}
	}
	return false
}
