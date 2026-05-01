package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"app/coordination"
)

func TestBackendPaginationComprehensive(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	s := NewSQLiteCoordinationStorage(db)
	groupID := "test-group"
	sender := "peer-1"

	// Helper to create a message with a specific hex hash
	saveMsg := func(gID string, content map[string]interface{}, wallTime int64) string {
		data, _ := json.Marshal(content)
		ts := coordination.HLCTimestamp{WallTimeMs: wallTime, Counter: 0, NodeID: sender}
		
		// Create a fake unique envelope hash
		h := sha256.New()
		h.Write([]byte(fmt.Sprintf("%s-%d", gID, wallTime)))
		if content["body"] != nil {
			h.Write([]byte(content["body"].(string)))
		}
		envHash := h.Sum(nil)
		msgID := hex.EncodeToString(envHash)

		err := s.SaveMessage(&coordination.StoredMessage{
			GroupID:      gID,
			Epoch:        1,
			SenderID:     "peer-1",
			Content:      data,
			Timestamp:    ts,
			EnvelopeHash: envHash,
		})
		if err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
		return msgID
	}

	// 1. Setup Data: Group A has 3 posts, Group B has 1 post
	// Post 1 (Group A) -> 3 comments
	// Post 2 (Group A) -> 0 comments
	// Post 3 (Group A) -> 1 comment (via parent_id)
	
	post1ID := saveMsg(groupID, map[string]interface{}{"type": "post", "body": "Post 1"}, 1001)
	post2ID := saveMsg(groupID, map[string]interface{}{"type": "post", "body": "Post 2"}, 1002)
	post3ID := saveMsg(groupID, map[string]interface{}{"type": "post", "body": "Post 3"}, 1003)
	
	otherGroup := "other-group"
	saveMsg(otherGroup, map[string]interface{}{"type": "post", "body": "Other Post"}, 1004)

	// Comments for Post 1
	saveMsg(groupID, map[string]interface{}{"type": "comment", "post_id": post1ID, "body": "C1"}, 2001)
	saveMsg(groupID, map[string]interface{}{"type": "comment", "post_id": post1ID, "body": "C2"}, 2002)
	saveMsg(groupID, map[string]interface{}{"type": "reply", "post_id": post1ID, "body": "C3"}, 2003)

	// Comment for Post 3 via parent_id (legacy support)
	saveMsg(groupID, map[string]interface{}{"type": "comment", "parent_id": post3ID, "body": "C4"}, 2004)

	// Random message (not a post or comment)
	saveMsg(groupID, map[string]interface{}{"type": "chat", "body": "Hello"}, 3001)

	t.Run("GetPostsPaginated_CountVerification", func(t *testing.T) {
		posts, err := s.GetPostsPaginated(groupID, 10, 0)
		if err != nil {
			t.Fatalf("GetPostsPaginated: %v", err)
		}
		// Should have 3 posts (Post 1, 2, 3). Order is DESC wall time.
		// Index 0: Post 3 (1003)
		// Index 1: Post 2 (1002)
		// Index 2: Post 1 (1001)
		if len(posts) != 3 {
			t.Errorf("expected 3 posts, got %d", len(posts))
		}

		for _, p := range posts {
			mID := hex.EncodeToString(p.EnvelopeHash)
			switch mID {
			case post1ID:
				if p.CommentCount != 3 {
					t.Errorf("Post 1: expected 3 comments, got %d", p.CommentCount)
				}
			case post2ID:
				if p.CommentCount != 0 {
					t.Errorf("Post 2: expected 0 comments, got %d", p.CommentCount)
				}
			case post3ID:
				if p.CommentCount != 1 {
					t.Errorf("Post 3: expected 1 comment, got %d", p.CommentCount)
				}
			}
		}
	})

	t.Run("GetPostsPaginated_Isolation", func(t *testing.T) {
		// Other group should only see its 1 post
		posts, err := s.GetPostsPaginated(otherGroup, 10, 0)
		if err != nil {
			t.Fatalf("GetPostsPaginated other: %v", err)
		}
		if len(posts) != 1 {
			t.Errorf("expected 1 post in other group, got %d", len(posts))
		}
	})

	t.Run("GetPostsPaginated_Pagination", func(t *testing.T) {
		// Page 1: limit 2
		posts1, err := s.GetPostsPaginated(groupID, 2, 0)
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if len(posts1) != 2 {
			t.Errorf("page 1: expected 2, got %d", len(posts1))
		}
		// Page 2: limit 2, offset 2
		posts2, err := s.GetPostsPaginated(groupID, 2, 2)
		if err != nil {
			t.Fatalf("page 2: %v", err)
		}
		if len(posts2) != 1 {
			t.Errorf("page 2: expected 1, got %d", len(posts2))
		}
	})

	t.Run("GetCommentsPaginated", func(t *testing.T) {
		comments, err := s.GetCommentsPaginated(groupID, post1ID, 10, 0)
		if err != nil {
			t.Fatalf("GetCommentsPaginated: %v", err)
		}
		if len(comments) != 3 {
			t.Errorf("expected 3 comments, got %d", len(comments))
		}
		
		// Test offset
		comments, err = s.GetCommentsPaginated(groupID, post1ID, 1, 1)
		if len(comments) != 1 {
			t.Errorf("expected 1 comment at offset 1, got %d", len(comments))
		}
	})

	t.Run("GetMessagesPaginated_GeneralChat", func(t *testing.T) {
		// This should return ALL messages in the group, paginated
		// Posts (3) + Comments (4) + Chat (1) = 8
		msgs, err := s.GetMessagesPaginated(groupID, 10, 0)
		if err != nil {
			t.Fatalf("GetMessagesPaginated: %v", err)
		}
		if len(msgs) != 8 {
			t.Errorf("expected 8 total messages, got %d", len(msgs))
		}
	})
}
