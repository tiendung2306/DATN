package db

import "fmt"

type BackupGroupRecord struct {
	GroupID    string
	GroupState []byte
	Epoch      uint64
	TreeHash   []byte
	MyRole     string
}

type BackupStoredMessage struct {
	GroupID       string
	Epoch         uint64
	SenderID      string
	Content       []byte
	HLCWallTimeMs int64
	HLCCounter    uint32
	HLCNodeID     string
}

type BackupKPBundle struct {
	PeerID        string
	PublicKP      []byte
	PrivateBundle []byte
}

type BackupPendingWelcome struct {
	TargetPeerID string
	GroupID      string
	WelcomeBytes []byte
}

func (d *Database) GetAllGroupsForBackup() ([]BackupGroupRecord, error) {
	rows, err := d.Conn.Query(`SELECT group_id, group_state, epoch, tree_hash, my_role FROM mls_groups`)
	if err != nil {
		return nil, fmt.Errorf("GetAllGroupsForBackup: %w", err)
	}
	defer rows.Close()

	var out []BackupGroupRecord
	for rows.Next() {
		var rec BackupGroupRecord
		if err := rows.Scan(&rec.GroupID, &rec.GroupState, &rec.Epoch, &rec.TreeHash, &rec.MyRole); err != nil {
			return nil, fmt.Errorf("GetAllGroupsForBackup scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) GetAllStoredMessagesForBackup() ([]BackupStoredMessage, error) {
	rows, err := d.Conn.Query(
		`SELECT group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id
		 FROM stored_messages
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllStoredMessagesForBackup: %w", err)
	}
	defer rows.Close()

	var out []BackupStoredMessage
	for rows.Next() {
		var msg BackupStoredMessage
		if err := rows.Scan(
			&msg.GroupID,
			&msg.Epoch,
			&msg.SenderID,
			&msg.Content,
			&msg.HLCWallTimeMs,
			&msg.HLCCounter,
			&msg.HLCNodeID,
		); err != nil {
			return nil, fmt.Errorf("GetAllStoredMessagesForBackup scan: %w", err)
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (d *Database) GetAllKPBundlesForBackup() ([]BackupKPBundle, error) {
	rows, err := d.Conn.Query(`SELECT peer_id, public_kp, private_bundle FROM kp_bundles`)
	if err != nil {
		return nil, fmt.Errorf("GetAllKPBundlesForBackup: %w", err)
	}
	defer rows.Close()

	var out []BackupKPBundle
	for rows.Next() {
		var rec BackupKPBundle
		if err := rows.Scan(&rec.PeerID, &rec.PublicKP, &rec.PrivateBundle); err != nil {
			return nil, fmt.Errorf("GetAllKPBundlesForBackup scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) GetAllPendingWelcomesForBackup() ([]BackupPendingWelcome, error) {
	rows, err := d.Conn.Query(`SELECT target_peer_id, group_id, welcome_bytes FROM pending_welcomes_out`)
	if err != nil {
		return nil, fmt.Errorf("GetAllPendingWelcomesForBackup: %w", err)
	}
	defer rows.Close()

	var out []BackupPendingWelcome
	for rows.Next() {
		var rec BackupPendingWelcome
		if err := rows.Scan(&rec.TargetPeerID, &rec.GroupID, &rec.WelcomeBytes); err != nil {
			return nil, fmt.Errorf("GetAllPendingWelcomesForBackup scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (d *Database) ClearApplicationDataForIdentityImport() error {
	// Clear volatile + content tables; identity tables are overwritten separately.
	queries := []string{
		`DELETE FROM coordination_state`,
		`DELETE FROM stored_messages`,
		`DELETE FROM kp_bundles`,
		`DELETE FROM pending_welcomes_out`,
		`DELETE FROM mls_groups`,
	}
	for _, q := range queries {
		if _, err := d.Conn.Exec(q); err != nil {
			return fmt.Errorf("ClearApplicationDataForIdentityImport: %w", err)
		}
	}
	return nil
}

func (d *Database) RestoreGroupsFromBackup(groups []BackupGroupRecord) error {
	for _, g := range groups {
		_, err := d.Conn.Exec(
			`INSERT INTO mls_groups (group_id, group_state, epoch, tree_hash, my_role)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(group_id) DO UPDATE SET
			     group_state = excluded.group_state,
			     epoch = excluded.epoch,
			     tree_hash = excluded.tree_hash,
			     my_role = excluded.my_role,
			     updated_at = CURRENT_TIMESTAMP`,
			g.GroupID, g.GroupState, g.Epoch, g.TreeHash, g.MyRole,
		)
		if err != nil {
			return fmt.Errorf("RestoreGroupsFromBackup(%s): %w", g.GroupID, err)
		}
	}
	return nil
}

func (d *Database) RestoreStoredMessagesFromBackup(messages []BackupStoredMessage) error {
	for _, m := range messages {
		_, err := d.Conn.Exec(
			`INSERT INTO stored_messages (group_id, epoch, sender_id, content, hlc_wall_time_ms, hlc_counter, hlc_node_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.GroupID, m.Epoch, m.SenderID, m.Content, m.HLCWallTimeMs, m.HLCCounter, m.HLCNodeID,
		)
		if err != nil {
			return fmt.Errorf("RestoreStoredMessagesFromBackup(%s): %w", m.GroupID, err)
		}
	}
	return nil
}

func (d *Database) RestoreKPBundlesFromBackup(records []BackupKPBundle) error {
	for _, r := range records {
		if err := d.SaveKPBundle(r.PeerID, r.PublicKP, r.PrivateBundle); err != nil {
			return fmt.Errorf("RestoreKPBundlesFromBackup(%s): %w", r.PeerID, err)
		}
	}
	return nil
}

func (d *Database) RestorePendingWelcomesFromBackup(records []BackupPendingWelcome) error {
	for _, r := range records {
		if err := d.SavePendingWelcome(r.TargetPeerID, r.GroupID, r.WelcomeBytes); err != nil {
			return fmt.Errorf("RestorePendingWelcomesFromBackup(%s,%s): %w", r.TargetPeerID, r.GroupID, err)
		}
	}
	return nil
}
