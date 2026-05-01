package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"app/adapter/store"
	"app/coordination"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestRuntimePaginationComprehensive(t *testing.T) {
	db, _ := store.InitDB(":memory:")
	defer db.Close()
	cs := store.NewSQLiteCoordinationStorage(db)
	
	rt := &Runtime{
		db:           db,
		coordStorage: cs,
	}

	groupID := "group-1"
	sender := "peer-1"

	saveMsg := func(gID string, content map[string]interface{}, wallTime int64) string {
		data, _ := json.Marshal(content)
		ts := coordination.HLCTimestamp{WallTimeMs: wallTime, Counter: 0, NodeID: sender}
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s-%d", gID, wallTime)))
		envHash := h.Sum(nil)
		msgID := hex.EncodeToString(envHash)

		cs.SaveMessage(&coordination.StoredMessage{
			GroupID:      gID,
			Epoch:        1,
			SenderID:     peer.ID(sender),
			Content:      data,
			Timestamp:    ts,
			EnvelopeHash: envHash,
		})
		return msgID
	}

	// Setup: 1 post with 5 comments
	postID := saveMsg(groupID, map[string]interface{}{"type": "post", "body": "Post 1"}, 1000)
	for i := 1; i <= 5; i++ {
		saveMsg(groupID, map[string]interface{}{
			"type":    "comment",
			"post_id": postID,
			"body":    "C",
		}, int64(2000+i))
	}

	t.Run("GetGroupPosts", func(t *testing.T) {
		posts, err := rt.GetGroupPosts(groupID, 10, 0)
		if err != nil {
			t.Fatalf("GetGroupPosts: %v", err)
		}
		if len(posts) != 1 {
			t.Fatalf("expected 1 post, got %d", len(posts))
		}
		if posts[0].CommentCount != 5 {
			t.Errorf("expected 5 comments, got %d", posts[0].CommentCount)
		}
		if posts[0].MessageID != postID {
			t.Errorf("expected messageID %s, got %s", postID, posts[0].MessageID)
		}
	})

	t.Run("GetPostComments", func(t *testing.T) {
		comments, err := rt.GetPostComments(groupID, postID, 2, 0)
		if err != nil {
			t.Fatalf("GetPostComments: %v", err)
		}
		if len(comments) != 2 {
			t.Errorf("expected 2 comments, got %d", len(comments))
		}
	})
}
