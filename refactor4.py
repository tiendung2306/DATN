import sys

def main():
    path = "app/coordination/coordinator_batch.go"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()

    # Fix storageKey derivation
    content = content.replace("storageKey, err := c.deriveLocalStorageKey()", "storageKey := deriveStorageKey(c.signingKey)")
    content = content.replace("""	if err != nil {
		slog.Error("Failed to derive storage key for batch replay", "error", err)
		return 0
	}""", "")

    # Fix StoredMessage and ApplicationEvent initialization
    content = content.replace("SenderID:     sender.String(),", "SenderID:     sender,")

    # The other fields like OriginalEpoch were already correct in refactor3.py!
    # Wait, let me double check.

    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

    test_path = "app/coordination/fork_heal_bidirectional_batching_test.go"
    with open(test_path, "r", encoding="utf-8") as f:
        test_content = f.read()

    test_content = test_content.replace("coord.dispatchMessageLocked(envBytes)", "coord.handleRawMessage(sender.id, envBytes)")

    with open(test_path, "w", encoding="utf-8") as f:
        f.write(test_content)

if __name__ == "__main__":
    main()
