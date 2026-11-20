package profile_test

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/types"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
)

type mockModel struct{}

func (mockModel) Settings() agent.ModelSettings { return nil }
func (mockModel) Complete(context.Context, []agent.Tool, []agent.Message) (*agent.Completion, error) {
	return &agent.Completion{Done: true}, nil
}
func (mockModel) ContextLimit() int64                   { return 0 }
func (mockModel) SupportedModalities() []agent.Modality { return nil }

type mockModelRegistry struct {
	models map[string]agent.Model
}

func newMockModelRegistry(names ...string) *mockModelRegistry {
	registry := &mockModelRegistry{models: map[string]agent.Model{}}
	for _, name := range names {
		registry.models[name] = mockModel{}
	}
	return registry
}

func (r *mockModelRegistry) Get(name string) (agent.Model, error) {
	model, ok := r.models[name]
	if !ok {
		return nil, types.ErrIsNotExist
	}
	return model, nil
}

type mockToolServer struct{}

func (mockToolServer) Tools() []agent.Tool { return nil }

type mockToolRegistry struct {
	servers map[string]agent.ToolServer
}

func newMockToolRegistry(names ...string) *mockToolRegistry {
	registry := &mockToolRegistry{servers: map[string]agent.ToolServer{}}
	for _, name := range names {
		registry.servers[name] = mockToolServer{}
	}
	return registry
}

func (r *mockToolRegistry) ToolServers(names ...string) ([]agent.ToolServer, error) {
	servers := make([]agent.ToolServer, 0, len(names))
	for _, name := range names {
		server, ok := r.servers[name]
		if !ok {
			return nil, types.ErrIsNotExist
		}
		servers = append(servers, server)
	}
	return servers, nil
}

type mockRepo struct {
	mu       sync.Mutex
	profiles map[profile.ID]profile.Profile
	saves    []profile.Profile
	deletes  []profile.ID

	allErr    error
	saveErr   error
	deleteErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{profiles: map[profile.ID]profile.Profile{}}
}

func (r *mockRepo) seed(p profile.Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.profiles[p.ID()] = p
}

func (r *mockRepo) All() ([]profile.Profile, error) {
	if r.allErr != nil {
		return nil, r.allErr
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	profiles := make([]profile.Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func (r *mockRepo) Get(id profile.ID) (profile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.profiles[id]
	if !ok {
		return nil, types.ErrIsNotExist
	}
	return p, nil
}

func (r *mockRepo) Save(p profile.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.saves = append(r.saves, p)
	if r.saveErr != nil {
		return r.saveErr
	}

	r.profiles[p.ID()] = p
	return nil
}

func (r *mockRepo) Delete(id profile.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.deletes = append(r.deletes, id)
	if r.deleteErr != nil {
		return r.deleteErr
	}

	delete(r.profiles, id)
	return nil
}

func (r *mockRepo) savedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.saves)
}

func (r *mockRepo) savedProfiles() []profile.Profile {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]profile.Profile(nil), r.saves...)
}

func (r *mockRepo) deletedIDs() []profile.ID {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]profile.ID(nil), r.deletes...)
}

type mockSync struct {
	mu    sync.Mutex
	err   error
	calls []profile.ID
}

func (s *mockSync) DeleteProfile(id profile.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, id)
	return s.err
}

func (s *mockSync) calledWith() []profile.ID {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]profile.ID(nil), s.calls...)
}

func newTestService(
	t *testing.T,
	storage *mockRepo,
	models *mockModelRegistry,
	tools *mockToolRegistry,
	syncs ...profile.Sync,
) *profile.Service {
	t.Helper()

	svc, err := profile.NewService(tools, models, storage, syncs)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func newProfile(id profile.ID) profile.Profile {
	return profile.NewProfile(id, "description", "system prompt", "model-a", []string{"filesystem"}, true)
}

func TestNewService_CreatesDefaultProfileWhenEmpty(t *testing.T) {
	storage := newMockRepo()
	svc, err := profile.NewService(
		newMockToolRegistry("filesystem"),
		newMockModelRegistry("model-a"),
		storage,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	saved := storage.savedProfiles()
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved profile, got %d", len(saved))
	}
	if saved[0].ID() != profile.ID("default") {
		t.Fatalf("expected default profile, got %s", saved[0].ID())
	}

	profiles, err := svc.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID() != profile.ID("default") {
		t.Fatalf("expected only default profile, got %v", profiles)
	}
}

func TestNewService_KeepsExistingProfiles(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("existing"))

	svc, err := profile.NewService(
		newMockToolRegistry("filesystem"),
		newMockModelRegistry("model-a"),
		storage,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no default profile saved, got %d saves", got)
	}

	profiles, err := svc.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID() != profile.ID("existing") {
		t.Fatalf("expected only existing profile, got %v", profiles)
	}
}

func TestNewService_FailsWhenListingProfilesFails(t *testing.T) {
	wantErr := errors.New("list failed")
	storage := newMockRepo()
	storage.allErr = wantErr

	_, err := profile.NewService(
		newMockToolRegistry("filesystem"),
		newMockModelRegistry("model-a"),
		storage,
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewService() error = %v, want %v", err, wantErr)
	}
}

func TestNewService_FailsWhenDefaultProfileSaveFails(t *testing.T) {
	wantErr := errors.New("save failed")
	storage := newMockRepo()
	storage.saveErr = wantErr

	_, err := profile.NewService(
		newMockToolRegistry("filesystem"),
		newMockModelRegistry("model-a"),
		storage,
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewService() error = %v, want %v", err, wantErr)
	}
}

func TestSave_PersistsValidProfile(t *testing.T) {
	storage := newMockRepo()
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	p := newProfile("writer")
	if err := svc.Save(p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := svc.Get("writer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID() != p.ID() {
		t.Fatalf("expected profile %s, got %s", p.ID(), got.ID())
	}
}

func TestSave_RejectsUnknownModel(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("existing"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	p := profile.NewProfile("writer", "description", "system prompt", "ghost-model", []string{"filesystem"}, false)
	if err := svc.Save(p); err == nil {
		t.Fatal("expected error for unknown model")
	}
	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no save, got %d", got)
	}
}

func TestSave_RejectsUnknownToolServer(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("existing"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	p := profile.NewProfile("writer", "description", "system prompt", "model-a", []string{"filesystem", "ghost"}, false)
	if err := svc.Save(p); err == nil {
		t.Fatal("expected error for unknown tool server")
	}
	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no save, got %d", got)
	}
}

func TestSave_SameIDOverwritesExistingProfile(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	updated := profile.NewProfile(
		"writer",
		"updated description",
		"new prompt",
		"model-a",
		[]string{"filesystem"},
		false,
	)
	if err := svc.Save(updated); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := svc.Get("writer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Description() != "updated description" {
		t.Fatalf("expected updated profile, got %q", got.Description())
	}

	profiles, err := svc.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected a single profile per id, got %d", len(profiles))
	}
}

func TestDelete_DeletesFromStorageAndSyncs(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	sync1 := &mockSync{}
	sync2 := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"), sync1, sync2)

	if err := svc.Delete("writer"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if got := storage.deletedIDs(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected storage delete of writer, got %v", got)
	}
	if got := sync1.calledWith(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected sync1 delete of writer, got %v", got)
	}
	if got := sync2.calledWith(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected sync2 delete of writer, got %v", got)
	}
}

func TestDelete_RejectsUnknownProfile(t *testing.T) {
	storage := newMockRepo()
	sync := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"), sync)

	if err := svc.Delete("missing"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if got := storage.deletedIDs(); len(got) != 0 {
		t.Fatalf("expected no storage delete, got %v", got)
	}
	if got := sync.calledWith(); len(got) != 0 {
		t.Fatalf("expected no sync calls, got %v", got)
	}
}

func TestDelete_StorageErrorSkipsSyncs(t *testing.T) {
	wantErr := errors.New("delete failed")
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	storage.deleteErr = wantErr
	sync := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"), sync)

	err := svc.Delete("writer")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Delete() error = %v, want %v", err, wantErr)
	}
	if got := sync.calledWith(); len(got) != 0 {
		t.Fatalf("expected no sync calls, got %v", got)
	}
}

func TestDelete_ContinuesSyncsAfterError(t *testing.T) {
	syncErr := errors.New("sync failed")
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	sync1 := &mockSync{err: syncErr}
	sync2 := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"), sync1, sync2)

	err := svc.Delete("writer")
	if !errors.Is(err, syncErr) {
		t.Fatalf("Delete() error = %v, want sync error", err)
	}
	if got := sync1.calledWith(); len(got) != 1 {
		t.Fatalf("expected sync1 to be called once, got %v", got)
	}
	if got := sync2.calledWith(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected sync2 to be called despite sync1 error, got %v", got)
	}
}

func TestDelete_JoinsSyncErrors(t *testing.T) {
	err1 := errors.New("sync 1 failed")
	err2 := errors.New("sync 2 failed")
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	svc := newTestService(
		t,
		storage,
		newMockModelRegistry("model-a"),
		newMockToolRegistry("filesystem"),
		&mockSync{err: err1},
		&mockSync{err: err2},
	)

	err := svc.Delete("writer")
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Fatalf("Delete() error = %v, want both sync errors joined", err)
	}
}

func TestDelete_WithoutSyncs(t *testing.T) {
	storage := newMockRepo()
	storage.seed(newProfile("writer"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if err := svc.Delete("writer"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := storage.deletedIDs(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected storage delete of writer, got %v", got)
	}
}
