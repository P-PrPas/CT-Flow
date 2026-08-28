// Package store is label/box storage backed by PostgreSQL (T-21/T-22) -- the
// DB-native replacement for labels/*.txt + classes.txt + testset.json.
//
// Ported from the FastAPI service's services/annotations_db.py and services/db.py. The SQL is
// copied statement for statement rather than rewritten: this schema and these
// queries are what the existing data was written by, and the concurrency
// guarantee in getOrCreateClass is the entire reason label storage moved to a
// database in the first place (docs/history/DB_MIGRATION_PLAN.md #4.1). Nothing here is
// an improvement on the Python; that is deliberate.
//
// One divergence, and it is not a rewrite so much as restoring what the Python
// had for free: reads resolve the project with projectID, not
// getOrCreateProject. Python called get_or_create_project on its own
// connection, which committed and dropped the row lock before the read ran.
// Wrapping the same statements in one Go transaction quietly turned every read
// into a writer queueing on the projects row -- same SQL, different isolation,
// and only the second one serialises a whole project behind one export. See
// getOrCreateProject.
//
// Prompt-bank embeddings are a separate concern and still live in a file, owned
// by the inference sidecar -- see backend/inference/bank.py.
//
// Two `kind`s share one schema: 'pool' (the working set, taught to the model)
// and 'testset' (held-out ground truth, never touches the bank). Each has its
// own class-index space, so a class named the same in both gets two different
// rows and two different idx values. Nothing here enforces the "never teach a
// test-set image" rule -- the handlers check IsTest() before writing, same as
// the routers always did.
//
// No ORM, matching the rest of this backend.
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaPath is backend/db/schema.sql -- the same file the Python service runs,
// read at startup rather than compiled in, for the same reason the checkpoint
// catalog is (internal/models): during the port both services apply it, and two
// copies of a schema that can disagree is a worse problem than a data file next
// to the binary. It is idempotent (CREATE TABLE IF NOT EXISTS), so running it on
// every start is safe and there is no separate migration step.

// Box is the shape every endpoint uses, in source-image pixels, never
// normalised (docs/API_REFERENCE.md).
type Box struct {
	Cls string     `json:"cls"`
	Box [4]float64 `json:"box"`
}

// Kind is which index space and image set a row belongs to.
const (
	KindPool    = "pool"
	KindTestset = "testset"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// InitSchema applies schemaPath. Idempotent, so it runs on every boot rather
// than requiring a migration step -- same as the FastAPI startup hook did.
func (s *Store) InitSchema(ctx context.Context, schemaPath string) error {
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("reading schema %s: %w", schemaPath, err)
	}
	_, err = s.pool.Exec(ctx, string(schema))
	return err
}

// tx runs fn inside one transaction: committed on a clean return, rolled back
// on error. Every write below goes through it, so a mid-request failure cannot
// leave a class registered without its annotation, or an image row without the
// boxes it was supposed to get.
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, fn)
}

// getOrCreateProject is for write paths only. The upsert takes an exclusive lock
// on the projects row and, unlike the Python it was copied from, holds it for
// the rest of the transaction -- there, get_or_create_project ran on its own
// connection and committed before the caller did anything else.
//
// Read paths must therefore use projectID instead. Using this one would make
// every GET /api/boxes take a write lock on the project, so an export reading a
// whole project would block every label save for it, and two people working in
// one folder would queue behind each other for no reason. That is the opposite
// of what moving this storage into a database was for.
func getOrCreateProject(ctx context.Context, q pgx.Tx, inputDir string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO projects (input_dir) VALUES ($1)
		 ON CONFLICT (input_dir) DO UPDATE SET input_dir = EXCLUDED.input_dir
		 RETURNING id`, inputDir).Scan(&id)
	return id, err
}

// projectID resolves a project without creating or locking one.
//
// found=false is a normal state, not an error: a folder that has been opened but
// never labeled has no row yet, and every reader below answers that with an
// empty result -- exactly what it answered before, when the read created the row
// on the way past and then found nothing in it.
func projectID(ctx context.Context, q pgx.Tx, inputDir string) (int64, bool, error) {
	var id int64
	switch err := q.QueryRow(ctx,
		`SELECT id FROM projects WHERE input_dir=$1`, inputDir).Scan(&id); err {
	case nil:
		return id, true, nil
	case pgx.ErrNoRows:
		return 0, false, nil
	default:
		return 0, false, err
	}
}

// getOrCreateClass is append-only and race-safe.
//
// This is the payoff of the whole move to a database: two people teaching two
// different new class names to the same project at the same moment must get
// different indices, because a label's class column is a position in this list
// and a collision silently mislabels every row that follows.
//
// The plain SELECT first avoids taking the lock at all in the overwhelmingly
// common case of an already-known class; only a genuinely new name pays for
// serialising on the project row, and only against writers to that same
// project.
func getOrCreateClass(ctx context.Context, q pgx.Tx, pid int64, kind, name string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`SELECT id FROM classes WHERE project_id=$1 AND kind=$2 AND name=$3`,
		pid, kind, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}

	// Serialise everyone writing to this project before computing the next idx.
	var lockedID int64
	if err := q.QueryRow(ctx, `SELECT id FROM projects WHERE id=$1 FOR UPDATE`,
		pid).Scan(&lockedID); err != nil {
		return 0, err
	}
	// Re-check: another transaction may have created it while we waited.
	err = q.QueryRow(ctx,
		`SELECT id FROM classes WHERE project_id=$1 AND kind=$2 AND name=$3`,
		pid, kind, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}

	var idx int
	if err := q.QueryRow(ctx,
		`SELECT COALESCE(MAX(idx), -1) + 1 FROM classes WHERE project_id=$1 AND kind=$2`,
		pid, kind).Scan(&idx); err != nil {
		return 0, err
	}
	err = q.QueryRow(ctx,
		`INSERT INTO classes (project_id, kind, idx, name) VALUES ($1, $2, $3, $4) RETURNING id`,
		pid, kind, idx, name).Scan(&id)
	return id, err
}

func getOrCreateImage(ctx context.Context, q pgx.Tx, pid int64, kind, path string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx,
		`INSERT INTO images (project_id, kind, path) VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, kind, path) DO UPDATE SET path = EXCLUDED.path
		 RETURNING id`, pid, kind, path).Scan(&id)
	return id, err
}

func classNames(ctx context.Context, q pgx.Tx, pid int64, kind string) ([]string, error) {
	rows, err := q.Query(ctx,
		`SELECT name FROM classes WHERE project_id=$1 AND kind=$2 ORDER BY idx`, pid, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// Classes returns the project's class names in idx order, not alphabetical --
// append-only, the same contract classes.txt had.
func (s *Store) Classes(ctx context.Context, inputDir, kind string) ([]string, error) {
	out := []string{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		out, err = classNames(ctx, q, id, kind)
		return err
	})
	return out, err
}

func boxesFor(ctx context.Context, q pgx.Tx, pid int64, kind, imagePath string) ([]Box, error) {
	rows, err := q.Query(ctx,
		`SELECT c.name, a.x1, a.y1, a.x2, a.y2 FROM annotations a
		 JOIN images i ON i.id = a.image_id
		 JOIN classes c ON c.id = a.class_id
		 WHERE i.project_id=$1 AND i.kind=$2 AND i.path=$3
		 ORDER BY a.id`, pid, kind, imagePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Box{}
	for rows.Next() {
		var b Box
		if err := rows.Scan(&b.Cls, &b.Box[0], &b.Box[1], &b.Box[2], &b.Box[3]); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ReadBoxes returns this image's saved boxes, or an empty slice.
func (s *Store) ReadBoxes(ctx context.Context, inputDir, kind, imagePath string) ([]Box, error) {
	out := []Box{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		out, err = boxesFor(ctx, q, id, kind, imagePath)
		return err
	})
	return out, err
}

// WriteBoxes replaces this image's annotation set with boxes, or their union
// with what is already saved when merge is true -- the same replace-vs-merge
// contract /api/label, /api/relabel and /api/testset/label always had.
//
// boxes may be empty: that is a legitimate "the model was wrong about
// everything here". New class names are get-or-created, append-only.
//
// Deliberately does NOT touch images.status: a caller meaning "this image just
// got manually labeled" calls MarkLabeled itself, and relabel deliberately does
// not, same as before.
//
// Returns the project's class list for this kind after the write.
func (s *Store) WriteBoxes(ctx context.Context, inputDir, kind, imagePath string,
	boxes []Box, createdBy *string, merge bool) ([]string, error) {
	var names []string
	err := s.tx(ctx, func(q pgx.Tx) error {
		pid, err := getOrCreateProject(ctx, q, inputDir)
		if err != nil {
			return err
		}
		final := boxes
		if merge {
			existing, err := boxesFor(ctx, q, pid, kind, imagePath)
			if err != nil {
				return err
			}
			final = append(existing, boxes...)
		}
		imageID, err := getOrCreateImage(ctx, q, pid, kind, imagePath)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM annotations WHERE image_id=$1`, imageID); err != nil {
			return err
		}
		for _, b := range final {
			classID, err := getOrCreateClass(ctx, q, pid, kind, b.Cls)
			if err != nil {
				return err
			}
			if _, err := q.Exec(ctx,
				`INSERT INTO annotations (image_id, class_id, x1, y1, x2, y2, created_by)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				imageID, classID, b.Box[0], b.Box[1], b.Box[2], b.Box[3], createdBy); err != nil {
				return err
			}
		}
		names, err = classNames(ctx, q, pid, kind)
		return err
	})
	return names, err
}

func (s *Store) MarkLabeled(ctx context.Context, inputDir, kind, imagePath string) error {
	return s.tx(ctx, func(q pgx.Tx) error {
		pid, err := getOrCreateProject(ctx, q, inputDir)
		if err != nil {
			return err
		}
		imageID, err := getOrCreateImage(ctx, q, pid, kind, imagePath)
		if err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE images SET status='labeled' WHERE id=$1`, imageID)
		return err
	})
}

// MarkAuto flags pool images as model-written. Never downgrades an image that
// is already 'labeled' (by hand) or already 'auto' -- hence the status guard in
// the UPDATE rather than a bare assignment.
func (s *Store) MarkAuto(ctx context.Context, inputDir string, imagePaths []string) error {
	if len(imagePaths) == 0 {
		return nil
	}
	return s.tx(ctx, func(q pgx.Tx) error {
		pid, err := getOrCreateProject(ctx, q, inputDir)
		if err != nil {
			return err
		}
		for _, p := range imagePaths {
			imageID, err := getOrCreateImage(ctx, q, pid, KindPool, p)
			if err != nil {
				return err
			}
			if _, err := q.Exec(ctx,
				`UPDATE images SET status='auto' WHERE id=$1 AND status='unlabeled'`,
				imageID); err != nil {
				return err
			}
		}
		return nil
	})
}

// StatusLists is the database half of BankSummary; the sidecar supplies the
// other half and the handler joins them.
type StatusLists struct {
	Labeled []string `json:"labeled"`
	Auto    []string `json:"auto"`
}

func (s *Store) ListByStatus(ctx context.Context, inputDir, kind string) (StatusLists, error) {
	out := StatusLists{Labeled: []string{}, Auto: []string{}}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		rows, err := q.Query(ctx,
			`SELECT path, status FROM images
			 WHERE project_id=$1 AND kind=$2 AND status != 'unlabeled'
			 ORDER BY path`, id, kind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var path, status string
			if err := rows.Scan(&path, &status); err != nil {
				return err
			}
			switch status {
			case "labeled":
				out.Labeled = append(out.Labeled, path)
			case "auto":
				out.Auto = append(out.Auto, path)
			}
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) ListTestImages(ctx context.Context, inputDir string) ([]string, error) {
	out := []string{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		rows, err := q.Query(ctx,
			`SELECT path FROM images WHERE project_id=$1 AND kind='testset' ORDER BY path`,
			id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// IsTest reports whether this image is held out. Pool endpoints that would
// teach the bank must check it and refuse, or a test image silently stops
// measuring generalization.
func (s *Store) IsTest(ctx context.Context, inputDir, imagePath string) (bool, error) {
	isTest := false
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		var one int
		err = q.QueryRow(ctx,
			`SELECT 1 FROM images WHERE project_id=$1 AND kind='testset' AND path=$2`,
			id, imagePath).Scan(&one)
		switch err {
		case nil:
			isTest = true
			return nil
		case pgx.ErrNoRows:
			return nil
		default:
			return err
		}
	})
	return isTest, err
}

// MarkTest flags pool images as held-out. No file or row copy of the image --
// just a second images row under kind='testset' sharing the same path. Skips
// paths already flagged, so importing twice is a no-op. Returns what was added.
func (s *Store) MarkTest(ctx context.Context, inputDir string, imagePaths []string) ([]string, error) {
	added := []string{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		pid, err := getOrCreateProject(ctx, q, inputDir)
		if err != nil {
			return err
		}
		for _, p := range imagePaths {
			var one int
			err := q.QueryRow(ctx,
				`SELECT 1 FROM images WHERE project_id=$1 AND kind='testset' AND path=$2`,
				pid, p).Scan(&one)
			if err == nil {
				continue // already flagged
			}
			if err != pgx.ErrNoRows {
				return err
			}
			if _, err := q.Exec(ctx,
				`INSERT INTO images (project_id, kind, path) VALUES ($1, 'testset', $2)`,
				pid, p); err != nil {
				return err
			}
			added = append(added, p)
		}
		return nil
	})
	return added, err
}

// UnmarkTest drops images out of the test set: the images row and its ground
// truth go (ON DELETE CASCADE); the pool row and the image file itself are
// untouched, because there was never a copy to delete.
func (s *Store) UnmarkTest(ctx context.Context, inputDir string, imagePaths []string) ([]string, error) {
	removed := []string{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		pid, err := getOrCreateProject(ctx, q, inputDir)
		if err != nil {
			return err
		}
		for _, p := range imagePaths {
			var id int64
			err := q.QueryRow(ctx,
				`DELETE FROM images WHERE project_id=$1 AND kind='testset' AND path=$2 RETURNING id`,
				pid, p).Scan(&id)
			if err == pgx.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			removed = append(removed, p)
		}
		return nil
	})
	return removed, err
}

// LabeledStems is the stems of test-set images with at least one annotation --
// the database equivalent of "a labels/<stem>.txt file exists". Test set only;
// pool status is tracked through ListByStatus instead.
func (s *Store) LabeledStems(ctx context.Context, inputDir string) ([]string, error) {
	out := []string{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		rows, err := q.Query(ctx,
			`SELECT DISTINCT i.path FROM images i JOIN annotations a ON a.image_id = i.id
			 WHERE i.project_id=$1 AND i.kind='testset'`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			// Stem, not basename: two files differing only by extension collapse
			// to one entry, exactly as the Python set did.
			stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if !seen[stem] {
				seen[stem] = true
				out = append(out, stem)
			}
		}
		return rows.Err()
	})
	return out, err
}

// LoadAnnotations returns every image in `kind` that has at least one box --
// one query instead of N ReadBoxes calls. Used for evaluate's ground truth and
// for export.
func (s *Store) LoadAnnotations(ctx context.Context, inputDir, kind string) (map[string][]Box, error) {
	out := map[string][]Box{}
	err := s.tx(ctx, func(q pgx.Tx) error {
		id, found, err := projectID(ctx, q, inputDir)
		if err != nil || !found {
			return err
		}
		rows, err := q.Query(ctx,
			`SELECT i.path, c.name, a.x1, a.y1, a.x2, a.y2 FROM images i
			 JOIN annotations a ON a.image_id = i.id
			 JOIN classes c ON c.id = a.class_id
			 WHERE i.project_id=$1 AND i.kind=$2
			 ORDER BY i.path, a.id`, id, kind)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var path string
			var b Box
			if err := rows.Scan(&path, &b.Cls, &b.Box[0], &b.Box[1], &b.Box[2], &b.Box[3]); err != nil {
				return err
			}
			out[path] = append(out[path], b)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteProject drops a project and everything under it (classes, images,
// annotations all cascade). Test and dev convenience; not exposed over HTTP.
func (s *Store) DeleteProject(ctx context.Context, inputDir string) error {
	return s.tx(ctx, func(q pgx.Tx) error {
		_, err := q.Exec(ctx, `DELETE FROM projects WHERE input_dir=$1`, inputDir)
		return err
	})
}

// UpsertUser records who an OIDC subject is, so the `sub` sitting in
// annotations.created_by and in the prompt bank's labeled_by can still be read
// as a person's name a year from now. Called on every successful login, not
// only the first: the provider is the source of truth for the display name and
// email, and both change without CT-Flow being told.
//
// No transaction -- it is one statement, and the row it touches is not one any
// other statement here reads.
func (s *Store) UpsertUser(ctx context.Context, oid, username, email string) error {
	var mail *string
	if email != "" {
		mail = &email
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (oid, username, email) VALUES ($1, $2, $3)
		ON CONFLICT (oid) DO UPDATE
		   SET username = EXCLUDED.username, email = EXCLUDED.email, updated_at = now()`,
		oid, username, mail)
	return err
}
