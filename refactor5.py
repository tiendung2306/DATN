import sys

def main():
    test_path = "app/coordination/fork_heal_bidirectional_batching_test.go"
    with open(test_path, "r", encoding="utf-8") as f:
        test_content = f.read()

    # Insert network.DrainAll() after BroadcastAnnounce()
    test_content = test_content.replace(
        "winner.coord.BroadcastAnnounce()\n\ttime.Sleep(500 * time.Millisecond)",
        "winner.coord.BroadcastAnnounce()\n\tnetwork.DrainAll()\n\ttime.Sleep(500 * time.Millisecond)"
    )

    with open(test_path, "w", encoding="utf-8") as f:
        f.write(test_content)

if __name__ == "__main__":
    main()
