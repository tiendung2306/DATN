import sys

def main():
    test_path = "app/coordination/fork_heal_bidirectional_batching_test.go"
    with open(test_path, "r", encoding="utf-8") as f:
        test_content = f.read()

    # setupCluster should return clk
    test_content = test_content.replace(
        "nodes, network, _ := setupCluster(t, 3, ",
        "nodes, network, clk := setupCluster(t, 3, "
    )
    test_content = test_content.replace(
        "nodes, _, _ := setupCluster(t, 2, ",
        "nodes, _, clk := setupCluster(t, 2, "
    )
    test_content = test_content.replace(
        "nodes, _, _ := setupCluster(t, 3, ",
        "nodes, _, clk := setupCluster(t, 3, "
    )

    # Replace time.Sleep with clk.Advance AND time.Sleep
    # We still need time.Sleep for real goroutines to be scheduled, but we also advance FakeClock
    test_content = test_content.replace(
        "time.Sleep(100 * time.Millisecond)",
        "clk.Advance(100 * time.Millisecond)\n\ttime.Sleep(100 * time.Millisecond)"
    )
    test_content = test_content.replace(
        "time.Sleep(500 * time.Millisecond)",
        "clk.Advance(500 * time.Millisecond)\n\ttime.Sleep(500 * time.Millisecond)"
    )

    with open(test_path, "w", encoding="utf-8") as f:
        f.write(test_content)

if __name__ == "__main__":
    main()
