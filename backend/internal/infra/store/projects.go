package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Projects: the rows behind the home page (FR-43, FR-44, FR-50).
//
// `input_dir` is still the identity every other endpoint and every query in
// store.go uses. `id` exists so the UI has something to put in a URL that is not
// a server path -- addressing key here, storage key there, and the two never
// swap roles (docs/PHASE2_WORKSPACE.md #2, decision 6).

// TaskDetection is the only task_type that exists. Rejecting anything else is
// the point: a value nothing reads is a value nothing can honour, and a project
// stored as 'segmentation' today would be a lie until a module answers to it.
const TaskDetection = "detection"

// ErrProjectExists is a create against a folder that already has one. Never a
// reason to adopt it silently: whoever asked to create it does not know they are
// about to join someone else's work.
var ErrProjectExists = errors.New("project already exists for this folder")

// Project is one row, plus the owner's name resolved through users. OwnerOID is
// what is stored; OwnerName is what a person can read, and it is empty for an
// owner who has no users row (a legacy LABEL_TOOL_USERS login, which is the
// CI/dev path -- see docs/PHASE2_WORKSPACE.md #2, decision 8).
type Project struct {
	ID        int64         `json:"id"`
	InputDir  string        `json:"input_dir"`
	Name      string        `json:"name"`
	TaskType  string        `json:"task_type"`
	Owner     *Person       `json:"owner"`
	Labeled   int           `json:"labeled"`
	Auto      int           `json:"auto"`
	Members   []Contributor `json:"contributors"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type Person struct {
	OID  string `json:"oid"`
	Name string `json:"username"`
}

// Contributor is someone who actually labeled in this project, counted from
// annotations.created_by -- what happened, not who was invited. That is the
// whole reason there is no members table (docs/PHASE2_WORKSPACE.md #2,
// decision 5).
type Contributor struct {
	OID   string `json:"oid"`
	Name  string `json:"username"`
	Boxes int    `json:"boxes"`
}

// EnsureProject returns the project for inputDir, creating it if there is none.
//
// This is the only way a project comes into existence, and both callers are a
// person saying so: POST /api/projects, and POST /api/session opening a folder
// that has never been opened. Everything else uses requireProject and fails with
// ErrNoProject, which is what keeps every row named and owned.
//
// created tells the caller which happened, so POST /api/projects can refuse a
// duplicate (ErrProjectExists) while opening a session adopts one.
//
// The ON CONFLICT arm updates nothing that matters -- it is there to make the
// statement return the existing row instead of no rows, and to take the same row
// lock, so two people opening the same folder at the same moment cannot both
// insert.
func (s *Store) EnsureProject(ctx context.Context, inputDir, name, ownerOID, taskType string) (Project, bool, error) {
	var (
		p       Project
		created bool
		owner   *string
	)
	if ownerOID != "" {
		owner = &ownerOID
	}
	if taskType == "" {
		taskType = TaskDetection
	}
	err := s.tx(ctx, func(q pgx.Tx) error {
		var inserted bool
		var gotOwner *string
		err := q.QueryRow(ctx, `
			INSERT INTO projects (input_dir, name, owner_oid, task_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (input_dir) DO UPDATE SET input_dir = EXCLUDED.input_dir
			RETURNING id, input_dir, name, owner_oid, task_type, created_at, updated_at,
			          (xmax = 0) AS inserted`,
			inputDir, name, owner, taskType,
		).Scan(&p.ID, &p.InputDir, &p.Name, &gotOwner, &p.TaskType,
			&p.CreatedAt, &p.UpdatedAt, &inserted)
		if err != nil {
			return err
		}
		created = inserted
		return attachOwner(ctx, q, &p, gotOwner)
	})
	if err != nil {
		return Project{}, false, err
	}
	return p, created, nil
}

// ListProjects is the home page in two queries regardless of how many projects
// there are: one for the rows and their counts, one for every contributor.
//
// Counts come from images.status, never from listing the folder. A card showing
// "34 of 3,000" would mean a readdir per project on every page load, and the
// total belongs inside the project anyway (docs/PHASE2_WORKSPACE.md #4.1).
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	out := []Project{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		rows, err := q.Query(ctx, `
			SELECT p.id, p.input_dir, p.name, p.owner_oid, u.username, p.task_type,
			       p.created_at, p.updated_at,
			       COALESCE(st.labeled, 0), COALESCE(st.auto, 0)
			FROM projects p
			LEFT JOIN users u ON u.oid = p.owner_oid
			LEFT JOIN (
			    SELECT project_id,
			           COUNT(*) FILTER (WHERE status = 'labeled') AS labeled,
			           COUNT(*) FILTER (WHERE status = 'auto')    AS auto
			    FROM images WHERE kind = 'pool' GROUP BY project_id
			) st ON st.project_id = p.id
			ORDER BY p.updated_at DESC, p.id DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		byID := map[int64]int{}
		for rows.Next() {
			var p Project
			var oid, username *string
			if err := rows.Scan(&p.ID, &p.InputDir, &p.Name, &oid, &username, &p.TaskType,
				&p.CreatedAt, &p.UpdatedAt, &p.Labeled, &p.Auto); err != nil {
				return err
			}
			p.Owner = person(oid, username)
			p.Members = []Contributor{}
			byID[p.ID] = len(out)
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		return eachContributor(ctx, q, 0, func(pid int64, c Contributor) {
			if i, ok := byID[pid]; ok {
				out[i].Members = append(out[i].Members, c)
			}
		})
	})
	return out, err
}

// GetProject resolves one project by id -- what /p/{id} calls on mount to turn
// the URL into the input_dir every other endpoint wants. found=false is a 404,
// not an error.
func (s *Store) GetProject(ctx context.Context, id int64) (Project, bool, error) {
	return s.oneProject(ctx, `WHERE p.id = $1`, id)
}

// GetProjectByDir is the same lookup keyed the way the rest of the API is.
func (s *Store) GetProjectByDir(ctx context.Context, inputDir string) (Project, bool, error) {
	return s.oneProject(ctx, `WHERE p.input_dir = $1`, inputDir)
}

func (s *Store) oneProject(ctx context.Context, where string, arg any) (Project, bool, error) {
	var p Project
	found := false
	err := s.tx(ctx, func(q pgx.Tx) error {
		var oid, username *string
		err := q.QueryRow(ctx, `
			SELECT p.id, p.input_dir, p.name, p.owner_oid, u.username, p.task_type,
			       p.created_at, p.updated_at,
			       COALESCE(st.labeled, 0), COALESCE(st.auto, 0)
			FROM projects p
			LEFT JOIN users u ON u.oid = p.owner_oid
			LEFT JOIN (
			    SELECT project_id,
			           COUNT(*) FILTER (WHERE status = 'labeled') AS labeled,
			           COUNT(*) FILTER (WHERE status = 'auto')    AS auto
			    FROM images WHERE kind = 'pool' GROUP BY project_id
			) st ON st.project_id = p.id `+where,
			arg,
		).Scan(&p.ID, &p.InputDir, &p.Name, &oid, &username, &p.TaskType,
			&p.CreatedAt, &p.UpdatedAt, &p.Labeled, &p.Auto)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		p.Owner = person(oid, username)
		p.Members = []Contributor{}
		return eachContributor(ctx, q, p.ID, func(_ int64, c Contributor) {
			p.Members = append(p.Members, c)
		})
	})
	if err != nil {
		return Project{}, false, err
	}
	return p, found, nil
}

// UpdateProject renames a project and/or hands an unowned one an owner.
//
// input_dir and task_type are deliberately not updatable: pointing a project at
// a different folder is a different project, and changing its type would orphan
// every label under it.
//
// The two COALESCEs read in opposite directions on purpose. name takes the new
// value when one is given, so a rename works; owner_oid takes the *existing*
// one first, so ownerOID can only fill a NULL. Claiming is for a project nobody
// has claimed -- the other order would let anyone hand themselves someone
// else's work with one PATCH, which is a permission question Phase 2 does not
// answer (docs/PHASE2_WORKSPACE.md #2, decision 1). Either way an omitted field
// stays as it was, so a rename cannot clear an owner.
func (s *Store) UpdateProject(ctx context.Context, id int64, name, ownerOID *string) (Project, bool, error) {
	var p Project
	found := false
	err := s.tx(ctx, func(q pgx.Tx) error {
		var oid *string
		err := q.QueryRow(ctx, `
			UPDATE projects
			   SET name = COALESCE($2, name),
			       owner_oid = COALESCE(owner_oid, $3),
			       updated_at = now()
			 WHERE id = $1
			 RETURNING id, input_dir, name, owner_oid, task_type, created_at, updated_at`,
			id, name, ownerOID,
		).Scan(&p.ID, &p.InputDir, &p.Name, &oid, &p.TaskType, &p.CreatedAt, &p.UpdatedAt)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return attachOwner(ctx, q, &p, oid)
	})
	if err != nil {
		return Project{}, false, err
	}
	return p, found, nil
}

// DeleteProjectByID drops a project and everything under it (classes, images and
// annotations all cascade). Nothing on disk is touched -- not the images, not
// the prompt bank -- so this deletes the record of the work, not the work.
func (s *Store) DeleteProjectByID(ctx context.Context, id int64) (Project, bool, error) {
	p, found, err := s.GetProject(ctx, id)
	if err != nil || !found {
		return Project{}, false, err
	}
	err = s.tx(ctx, func(q pgx.Tx) error {
		_, err := q.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
		return err
	})
	return p, err == nil, err
}

// eachContributor walks annotations grouped by (project, author). pid 0 means
// every project, which is what lets ListProjects stay at two queries.
//
// created_by holds an OIDC `sub`, so the users join is what turns it back into a
// person; an author with no row there (legacy local login) falls back to the
// stored value rather than vanishing from the list.
func eachContributor(ctx context.Context, q pgx.Tx, pid int64, add func(int64, Contributor)) error {
	rows, err := q.Query(ctx, `
		SELECT i.project_id, a.created_by, u.username, COUNT(*)
		FROM annotations a
		JOIN images i ON i.id = a.image_id
		LEFT JOIN users u ON u.oid = a.created_by
		WHERE a.created_by IS NOT NULL AND ($1 = 0 OR i.project_id = $1)
		GROUP BY i.project_id, a.created_by, u.username
		ORDER BY COUNT(*) DESC, a.created_by`, pid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int64
		var oid string
		var username *string
		var boxes int
		if err := rows.Scan(&projectID, &oid, &username, &boxes); err != nil {
			return err
		}
		name := oid
		if username != nil {
			name = *username
		}
		add(projectID, Contributor{OID: oid, Name: name, Boxes: boxes})
	}
	return rows.Err()
}

func attachOwner(ctx context.Context, q pgx.Tx, p *Project, oid *string) error {
	p.Members = []Contributor{}
	if oid == nil {
		return nil
	}
	var username *string
	err := q.QueryRow(ctx, `SELECT username FROM users WHERE oid = $1`, *oid).Scan(&username)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	p.Owner = person(oid, username)
	return nil
}

// person keeps an owner readable when the users row is missing: the `sub` is
// still who it is, it just cannot be spelled. Returning nil for no owner at all
// is what the UI renders as "no owner".
func person(oid, username *string) *Person {
	if oid == nil {
		return nil
	}
	name := *oid
	if username != nil {
		name = *username
	}
	return &Person{OID: *oid, Name: name}
}
