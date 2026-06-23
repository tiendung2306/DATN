import sys

def main():
    path = "app/coordination/coordinator.go"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    bad_block = """							msgRec, err := c.storage.GetMessageByEnvelopeHash(record.EnvelopeHash)
							if err == nil && msgRec != nil {"""
    good_block = """							// We don't have GetMessageByEnvelopeHash, but we can fetch them
							ownMsgs, _ := c.storage.GetMessagesByOwnerInRange(c.groupID, c.localID.String(), 0, c.clock.Now().UnixMilli())
							var msgRec *StoredMessage
							for _, m := range ownMsgs {
								if bytes.Equal(m.EnvelopeHash, record.EnvelopeHash) {
									msgRec = m
									break
								}
							}
							if msgRec != nil {"""

    content = content.replace(bad_block, good_block)

    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

if __name__ == "__main__":
    main()
