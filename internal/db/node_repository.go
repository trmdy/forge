// Package db provides SQLite database access for Swarm.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/opencode-ai/swarm/internal/models"
)

// Common repository errors
var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrNodeAlreadyExists = errors.New("node with this name already exists")
)

// NodeRepository handles node persistence.
type NodeRepository struct {
	db *DB
}

// NewNodeRepository creates a new NodeRepository.
func NewNodeRepository(db *DB) *NodeRepository {
	return &NodeRepository{db: db}
}

// Create adds a new node to the database.
func (r *NodeRepository) Create(ctx context.Context, node *models.Node) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("invalid node: %w", err)
	}

	if node.ID == "" {
		node.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now

	metadataJSON, err := json.Marshal(node.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var lastSeen *string
	if node.LastSeen != nil {
		s := node.LastSeen.Format(time.RFC3339)
		lastSeen = &s
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO nodes (
			id, name, ssh_target, ssh_backend, ssh_key_path,
			ssh_agent_forwarding, ssh_proxy_jump, ssh_control_master,
			ssh_control_path, ssh_control_persist, ssh_timeout_seconds,
			status, is_local, last_seen_at, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		node.ID,
		node.Name,
		node.SSHTarget,
		string(node.SSHBackend),
		node.SSHKeyPath,
		boolToInt(node.SSHAgentForwarding),
		node.SSHProxyJump,
		node.SSHControlMaster,
		node.SSHControlPath,
		node.SSHControlPersist,
		node.SSHTimeoutSeconds,
		string(node.Status),
		boolToInt(node.IsLocal),
		lastSeen,
		string(metadataJSON),
		node.CreatedAt.Format(time.RFC3339),
		node.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrNodeAlreadyExists
		}
		return fmt.Errorf("failed to insert node: %w", err)
	}

	return nil
}

// Get retrieves a node by ID.
func (r *NodeRepository) Get(ctx context.Context, id string) (*models.Node, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			id, name, ssh_target, ssh_backend, ssh_key_path,
			ssh_agent_forwarding, ssh_proxy_jump, ssh_control_master, ssh_control_path,
			ssh_control_persist, ssh_timeout_seconds, status,
			is_local, last_seen_at, metadata_json, created_at, updated_at
		FROM nodes WHERE id = ?
	`, id)

	return r.scanNode(row)
}

// GetByName retrieves a node by name.
func (r *NodeRepository) GetByName(ctx context.Context, name string) (*models.Node, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			id, name, ssh_target, ssh_backend, ssh_key_path,
			ssh_agent_forwarding, ssh_proxy_jump, ssh_control_master, ssh_control_path,
			ssh_control_persist, ssh_timeout_seconds, status,
			is_local, last_seen_at, metadata_json, created_at, updated_at
		FROM nodes WHERE name = ?
	`, name)

	return r.scanNode(row)
}

// List retrieves all nodes, optionally filtered by status.
func (r *NodeRepository) List(ctx context.Context, status *models.NodeStatus) ([]*models.Node, error) {
	var rows *sql.Rows
	var err error

	if status != nil {
		rows, err = r.db.QueryContext(ctx, `
			SELECT 
				id, name, ssh_target, ssh_backend, ssh_key_path,
				ssh_agent_forwarding, ssh_proxy_jump, ssh_control_master, ssh_control_path,
				ssh_control_persist, ssh_timeout_seconds, status,
				is_local, last_seen_at, metadata_json, created_at, updated_at
			FROM nodes WHERE status = ?
			ORDER BY name
		`, string(*status))
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT 
				id, name, ssh_target, ssh_backend, ssh_key_path,
				ssh_agent_forwarding, ssh_proxy_jump, ssh_control_master, ssh_control_path,
				ssh_control_persist, ssh_timeout_seconds, status,
				is_local, last_seen_at, metadata_json, created_at, updated_at
			FROM nodes ORDER BY name
		`)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		node, err := r.scanNodeFromRows(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating nodes: %w", err)
	}

	return nodes, nil
}

// Update updates an existing node.
func (r *NodeRepository) Update(ctx context.Context, node *models.Node) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("invalid node: %w", err)
	}

	node.UpdatedAt = time.Now().UTC()

	metadataJSON, err := json.Marshal(node.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var lastSeen *string
	if node.LastSeen != nil {
		s := node.LastSeen.Format(time.RFC3339)
		lastSeen = &s
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE nodes SET
			name = ?,
			ssh_target = ?,
			ssh_backend = ?,
			ssh_key_path = ?,
			ssh_agent_forwarding = ?,
			ssh_proxy_jump = ?,
			ssh_control_master = ?,
			ssh_control_path = ?,
			ssh_control_persist = ?,
			ssh_timeout_seconds = ?,
			status = ?,
			is_local = ?,
			last_seen_at = ?,
			metadata_json = ?,
			updated_at = ?
		WHERE id = ?
	`,
		node.Name,
		node.SSHTarget,
		string(node.SSHBackend),
		node.SSHKeyPath,
		boolToInt(node.SSHAgentForwarding),
		node.SSHProxyJump,
		node.SSHControlMaster,
		node.SSHControlPath,
		node.SSHControlPersist,
		node.SSHTimeoutSeconds,
		string(node.Status),
		boolToInt(node.IsLocal),
		lastSeen,
		string(metadataJSON),
		node.UpdatedAt.Format(time.RFC3339),
		node.ID,
	)

	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrNodeAlreadyExists
		}
		return fmt.Errorf("failed to update node: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

// Delete removes a node by ID.
func (r *NodeRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM nodes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

// UpdateStatus updates only the status and last_seen fields.
func (r *NodeRepository) UpdateStatus(ctx context.Context, id string, status models.NodeStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.ExecContext(ctx, `
		UPDATE nodes SET status = ?, last_seen_at = ?, updated_at = ?
		WHERE id = ?
	`, string(status), now, now, id)

	if err != nil {
		return fmt.Errorf("failed to update node status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

// GetAgentCount returns the number of agents on a node.
func (r *NodeRepository) GetAgentCount(ctx context.Context, nodeID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agents a
		JOIN workspaces w ON a.workspace_id = w.id
		WHERE w.node_id = ?
	`, nodeID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count agents: %w", err)
	}

	return count, nil
}

// scanNode scans a single node from a row.
func (r *NodeRepository) scanNode(row *sql.Row) (*models.Node, error) {
	var node models.Node
	var sshBackend, status string
	var isLocal int
	var agentForwarding int
	var proxyJump, controlMaster, controlPath, controlPersist sql.NullString
	var timeoutSeconds sql.NullInt64
	var lastSeen, metadataJSON sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&node.ID,
		&node.Name,
		&node.SSHTarget,
		&sshBackend,
		&node.SSHKeyPath,
		&agentForwarding,
		&proxyJump,
		&controlMaster,
		&controlPath,
		&controlPersist,
		&timeoutSeconds,
		&status,
		&isLocal,
		&lastSeen,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to scan node: %w", err)
	}

	node.SSHBackend = models.SSHBackend(sshBackend)
	node.Status = models.NodeStatus(status)
	node.IsLocal = isLocal != 0
	node.SSHAgentForwarding = agentForwarding != 0
	if proxyJump.Valid {
		node.SSHProxyJump = proxyJump.String
	}
	if controlMaster.Valid {
		node.SSHControlMaster = controlMaster.String
	}
	if controlPath.Valid {
		node.SSHControlPath = controlPath.String
	}
	if controlPersist.Valid {
		node.SSHControlPersist = controlPersist.String
	}
	if timeoutSeconds.Valid {
		node.SSHTimeoutSeconds = int(timeoutSeconds.Int64)
	}

	if lastSeen.Valid {
		t, err := time.Parse(time.RFC3339, lastSeen.String)
		if err == nil {
			node.LastSeen = &t
		}
	}

	if metadataJSON.Valid {
		if err := json.Unmarshal([]byte(metadataJSON.String), &node.Metadata); err != nil {
			r.db.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to parse node metadata")
		}
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		node.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		node.UpdatedAt = t
	}

	return &node, nil
}

// scanNodeFromRows scans a single node from a rows iterator.
func (r *NodeRepository) scanNodeFromRows(rows *sql.Rows) (*models.Node, error) {
	var node models.Node
	var sshBackend, status string
	var isLocal int
	var agentForwarding int
	var proxyJump, controlMaster, controlPath, controlPersist sql.NullString
	var timeoutSeconds sql.NullInt64
	var lastSeen, metadataJSON sql.NullString
	var createdAt, updatedAt string

	err := rows.Scan(
		&node.ID,
		&node.Name,
		&node.SSHTarget,
		&sshBackend,
		&node.SSHKeyPath,
		&agentForwarding,
		&proxyJump,
		&controlMaster,
		&controlPath,
		&controlPersist,
		&timeoutSeconds,
		&status,
		&isLocal,
		&lastSeen,
		&metadataJSON,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan node: %w", err)
	}

	node.SSHBackend = models.SSHBackend(sshBackend)
	node.Status = models.NodeStatus(status)
	node.IsLocal = isLocal != 0
	node.SSHAgentForwarding = agentForwarding != 0
	if proxyJump.Valid {
		node.SSHProxyJump = proxyJump.String
	}
	if controlMaster.Valid {
		node.SSHControlMaster = controlMaster.String
	}
	if controlPath.Valid {
		node.SSHControlPath = controlPath.String
	}
	if controlPersist.Valid {
		node.SSHControlPersist = controlPersist.String
	}
	if timeoutSeconds.Valid {
		node.SSHTimeoutSeconds = int(timeoutSeconds.Int64)
	}

	if lastSeen.Valid {
		t, err := time.Parse(time.RFC3339, lastSeen.String)
		if err == nil {
			node.LastSeen = &t
		}
	}

	if metadataJSON.Valid {
		if err := json.Unmarshal([]byte(metadataJSON.String), &node.Metadata); err != nil {
			r.db.logger.Warn().Err(err).Str("node_id", node.ID).Msg("failed to parse node metadata")
		}
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		node.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		node.UpdatedAt = t
	}

	return &node, nil
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isUniqueConstraintError(err error) bool {
	// SQLite returns "UNIQUE constraint failed" errors
	// Be specific to avoid matching CHECK constraint errors
	return err != nil && contains(err.Error(), "UNIQUE constraint failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
