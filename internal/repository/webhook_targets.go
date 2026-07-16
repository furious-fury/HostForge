package repository

import (
	"context"
	"strings"
)

type AutoDeployTarget struct {
	ServiceID     string
	EnvironmentID string
	RepoURL       string
	Branch        string
}

func (s *Store) ListAutoDeployTargets(ctx context.Context) ([]AutoDeployTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT svc.id,se.environment_id,svc.repo_url,se.branch
		FROM services svc
		JOIN service_environments se ON se.service_id=svc.id
		WHERE se.auto_deploy=1 AND TRIM(se.branch)<>''
		ORDER BY svc.id,se.environment_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AutoDeployTarget, 0)
	for rows.Next() {
		var item AutoDeployTarget
		if err := rows.Scan(&item.ServiceID, &item.EnvironmentID, &item.RepoURL, &item.Branch); err != nil {
			return nil, err
		}
		item.RepoURL = strings.TrimSpace(item.RepoURL)
		item.Branch = strings.TrimSpace(item.Branch)
		out = append(out, item)
	}
	return out, rows.Err()
}
