import sqlite3
import json
import base64

db_path = "../.demo-control/runtimes/node-01/app.db"
conn = sqlite3.connect(db_path)
cursor = conn.cursor()

rows = cursor.execute("SELECT seq, epoch, envelope, source_path FROM envelope_log WHERE msg_type=\"commit\" ORDER BY seq ASC").fetchall()

for seq, epoch, envelope, source in rows:
    data = json.loads(envelope)
    payload = data.get("payload", {})
    added = payload.get("add_deliveries", [])
    proposals = payload.get("included_proposals", [])
    
    print(f"Seq {seq} | Epoch {epoch} -> {epoch+1} | Source: {source}")
    if added:
        for a in added:
            print(f"  + Added: {a.get(\"target_peer_id\")} (Op: {a.get(\"operation_id\")[:8]})")
    else:
        print(f"  * No members added (likely Key Rotation or internal state change)")
    
    # Check if it contains remove proposals
    removals = [p for p in proposals if p.get("proposal_type") == 1] # ProposalRemove is 1 usually, need to check
    if removals:
         print(f"  - Removals detected")

conn.close()
