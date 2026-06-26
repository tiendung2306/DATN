package coordination

// SetStateForTest directly sets the Coordinator's internal state for testing.
// This bypasses normal protocol flow and should only be used in tests.
func (c *Coordinator) SetStateForTest(epoch uint64, groupState []byte, treeHash []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch = epoch
	c.groupState = groupState
	c.treeHash = treeHash
	c.epochTracker = NewEpochTracker(epoch, treeHash)
	c.singleWriter = NewSingleWriter(c.activeView, c.localID, epoch, c.cfg)
	c.singleWriter.SetAuthorizedCommitters(c.groupID, c.authorizedCommitters)
	c.forkDetector.Reset()
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    copyBytes(treeHash),
		MemberCount: c.activeView.Size(),
		Epoch:       epoch,
	})
}

// ResetForkDetectorForTest resets the fork detector and updates local state.
// Used in benchmarks to clear stale known branches after SetStateForTest.
func (c *Coordinator) ResetForkDetectorForTest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forkDetector.Reset()
	c.forkDetector.UpdateLocal(GroupStateAnnouncement{
		TreeHash:    copyBytes(c.treeHash),
		MemberCount: c.activeView.Size(),
		Epoch:       c.epoch,
	})
}
