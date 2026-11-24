// Package migrations applies step-by-step migrations to the application's data folder.
//
// # Rules
//
// Migrations keep the on-disk data of a long-lived installation compatible with the current build.
// The rules below are not style preferences: they are the contract that keeps old installations alive.
//
//  1. Migrations are applied strictly sequentially, one after another, in the order listed in Apply.
//     Migration N+1 always runs after migration N has finished, so a migration may rely on the layout
//     produced by every previous migration. The order is never changed, skipped or parallelized.
//
//  2. Old migrations are never rewritten, edited or deleted.
//     Once a migration has shipped, the data it touches is already in the wild. If a migration turned out
//     to be wrong or incomplete, the fix is a NEW migration appended to the end of the sequence: it repairs
//     the damage on top of the old one. Editing history does not repair installations that already ran the
//     old code, it only hides the bug from the ones that did not run it yet.
//
//  3. Migrations must not depend on the application code.
//     The rest of the codebase is under active development: packages, types, constants, interfaces, file
//     names and config formats change all the time. A migration is a snapshot of the past and has to keep
//     compiling and behaving exactly as it did on the day it shipped, no matter how the application
//     evolves. This package therefore declares its own Storage port, its own constants, its own DTO
//     structs and its own file and folder names. Those are deliberate boilerplate copies of the layout as
//     it was at that moment. Never replace a local copy with an import from another package, and never
//     "reuse" an application helper just because it happens to do the same thing today.
//
//  4. Everything a migration touches is vendored inside this package: structures, constants, file names,
//     folder names and permission bits. If the real application later renames a folder, changes a config
//     format or drops an interface, migrations must keep working against the names they were written for.
//
//  5. All migrations run from a single entry point: Apply. It is the only function the application calls.
//     Migrations themselves are unexported and are never invoked directly, so the sequence lives in
//     exactly one place and cannot be split, reordered or bypassed by accident.
//
// # Layout
//
// Every migration lives in its own file named after its position in the sequence: 1_migration.go,
// 2_migration.go, and so on. Files are named by their order, not by their content: content names age
// badly and start to lie once a migration is superseded, while the number stays true. Each migration is a
// single function with the same shape:
//
//	func migrationN(root Storage, logger *slog.Logger) error
//
// To add a migration: create the next numbered file, implement the function there, and append it to the
// end of the list in Apply. Never touch the previous files.
//
// # Errors and idempotency
//
// Migrations are idempotent: they check whether the legacy layout is present and do nothing when it is
// not, so running them on a fresh or already migrated data folder is a no-op.
// Structural failures (cannot read the data folder) are returned to the caller and stop the sequence.
// Failures that only concern a single legacy object are logged and skipped, so one broken file cannot
// block the rest of the migration.
//
// This package is excluded from linters: migrations are frozen history, not living code.
package migrations

import (
	"fmt"
	"io/fs"
	"log/slog"
)

// Storage is the frozen subset of the file storage used by migrations.
// It duplicates the application port on purpose: migrations must not depend on code that can change.
type Storage interface {
	FS() fs.FS
	Stat(name string) (fs.FileInfo, error)
	MkdirAll(name string, perm fs.FileMode) error
	ReadFile(name string) ([]byte, error)
	Remove(name string) error
	Rename(oldname, newname string) error
	WriteFile(name string, data []byte, perm fs.FileMode) error
}

type migration struct {
	name string
	run  func(root Storage, logger *slog.Logger) error
}

// Apply runs every migration in sequence and stops at the first structural failure.
func Apply(root Storage, logger *slog.Logger) error {
	migrations := []migration{
		{name: "1: rename shared folder to repos", run: migration1},
		{name: "2: rename agent log to sys log", run: migration2},
		{name: "3: move configs into conf folder", run: migration3},
		{name: "4: move activity into root folder", run: migration4},
		{name: "5: move memory into root folder", run: migration5},
		{name: "6: move sessions into root folder", run: migration6},
		{name: "7: convert agent markdown into agent toml", run: migration7},
		{name: "8: clean agent folders", run: migration8},
		{name: "9: convert agent toml into profiles", run: migration9},
		{name: "10: clean root folder", run: migration10},
	}

	for _, m := range migrations {
		if err := m.run(root, logger); err != nil {
			return fmt.Errorf("%s: %w", m.name, err)
		}
	}

	return nil
}
