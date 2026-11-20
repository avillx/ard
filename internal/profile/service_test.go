package profile_test

import (
	"ard/internal/agent"
	"ard/internal/profile"
	"ard/internal/prompt"
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
	saves    []map[profile.ID]profile.Profile

	loadErr error
	saveErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{profiles: map[profile.ID]profile.Profile{}}
}

func (r *mockRepo) seed(p profile.Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.profiles[p.ID()] = p
}

func (r *mockRepo) Load() (map[profile.ID]profile.Profile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.loadErr != nil {
		return nil, r.loadErr
	}

	profiles := make(map[profile.ID]profile.Profile, len(r.profiles))
	for id, p := range r.profiles {
		profiles[id] = p
	}
	return profiles, nil
}

func (r *mockRepo) Save(profiles map[profile.ID]profile.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.saves = append(r.saves, profiles)
	if r.saveErr != nil {
		return r.saveErr
	}

	r.profiles = profiles
	return nil
}

func (r *mockRepo) savedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.saves)
}

func (r *mockRepo) storedIDs() []profile.ID {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]profile.ID, 0, len(r.profiles))
	for id := range r.profiles {
		ids = append(ids, id)
	}
	return ids
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
) *profile.Service {
	t.Helper()

	svc, err := profile.NewService(tools, models, storage)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func newTestStorage(t *testing.T, profiles ...profile.Profile) *mockRepo {
	t.Helper()

	storage := newMockRepo()
	for _, p := range profiles {
		storage.seed(p)
	}
	return storage
}

func newProfile(id profile.ID) profile.Profile {
	return profile.NewProfile(id, "description", "system prompt", "model-a", []string{"filesystem"}, true)
}

func assertProfile(t *testing.T, got, want profile.Profile) {
	t.Helper()

	if got.ID() != want.ID() {
		t.Fatalf("expected id %q, got %q", want.ID(), got.ID())
	}
	if got.Description() != want.Description() {
		t.Fatalf("expected description %q, got %q", want.Description(), got.Description())
	}
	if got.SystemPrompt() != want.SystemPrompt() {
		t.Fatalf("expected system prompt %q, got %q", want.SystemPrompt(), got.SystemPrompt())
	}
	if got.Model() != want.Model() {
		t.Fatalf("expected model %q, got %q", want.Model(), got.Model())
	}
	if !slices.Equal(got.ToolServers(), want.ToolServers()) {
		t.Fatalf("expected tool servers %v, got %v", want.ToolServers(), got.ToolServers())
	}
	if got.HasMemory() != want.HasMemory() {
		t.Fatalf("expected memory %v, got %v", want.HasMemory(), got.HasMemory())
	}
}

func TestNewService_DoesNotPersistDefaultProfile(t *testing.T) {
	storage := newMockRepo()
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no profile saves, got %d", got)
	}

	profiles, err := svc.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected no stored profiles, got %v", profiles)
	}
}

func TestNewService_DoesNotTouchStorage(t *testing.T) {
	storage := newMockRepo()
	storage.loadErr = errors.New("load failed")
	storage.saveErr = errors.New("save failed")

	svc, err := profile.NewService(
		newMockToolRegistry("filesystem"),
		newMockModelRegistry("model-a"),
		storage,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v, want nil", err)
	}
	if svc == nil {
		t.Fatal("expected service, got nil")
	}
}

func TestAll_ReturnsStoredProfiles(t *testing.T) {
	storage := newTestStorage(t, newProfile("writer"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	profiles, err := svc.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID() != profile.ID("writer") {
		t.Fatalf("expected only stored writer profile, got %v", profiles)
	}
}

func TestAll_PropagatesStorageError(t *testing.T) {
	wantErr := errors.New("load failed")
	storage := newMockRepo()
	storage.loadErr = wantErr
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if _, err := svc.All(); !errors.Is(err, wantErr) {
		t.Fatalf("All() error = %v, want %v", err, wantErr)
	}
}

func TestGet_ReturnsDefaultProfile(t *testing.T) {
	storage := newTestStorage(t)
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	want := profile.NewProfile(
		profile.DefaultProfileID,
		profile.DefaultProfileDescription,
		prompt.DefaultAgentProfile(),
		profile.AnyModel,
		[]string{profile.FileSystem},
		profile.DefaultMemoryState,
	)

	for _, id := range []profile.ID{"", profile.DefaultProfileID} {
		got, err := svc.Get(id)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", id, err)
		}
		assertProfile(t, got, want)
	}
}

func TestGet_PatchesStoredProfileOverDefault(t *testing.T) {
	storage := newTestStorage(t, profile.NewProfile(
		"writer",
		"writer description",
		"writer prompt",
		"model-b",
		[]string{"filesystem", "shell"},
		true,
	))
	svc := newTestService(
		t,
		storage,
		newMockModelRegistry("model-a", "model-b"),
		newMockToolRegistry("filesystem", "shell"),
	)

	got, err := svc.Get("writer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	assertProfile(t, got, profile.NewProfile(
		"writer",
		"writer description",
		"writer prompt",
		"model-b",
		[]string{"filesystem", "shell"},
		true,
	))
}

func TestGet_KeepsDefaultsForOmittedFields(t *testing.T) {
	storage := newTestStorage(t, profile.NewProfile("writer", "writer description", "", "", nil, false))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	got, err := svc.Get("writer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	assertProfile(t, got, profile.NewProfile(
		"writer",
		"writer description",
		prompt.DefaultAgentProfile(),
		profile.AnyModel,
		[]string{profile.FileSystem},
		profile.DefaultMemoryState,
	))
}

func TestGet_ReturnsErrorForUnknownProfile(t *testing.T) {
	storage := newTestStorage(t)
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if _, err := svc.Get("missing"); !errors.Is(err, types.ErrIsNotExist) {
		t.Fatalf("Get() error = %v, want %v", err, types.ErrIsNotExist)
	}
}

func TestGet_PropagatesSelectedProfileError(t *testing.T) {
	wantErr := errors.New("load failed")
	storage := newMockRepo()
	storage.loadErr = wantErr
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if _, err := svc.Get("writer"); !errors.Is(err, wantErr) {
		t.Fatalf("Get() error = %v, want %v", err, wantErr)
	}
}

func TestGet_MergesGlobalPromptIntoDefaultProfile(t *testing.T) {
	storage := newTestStorage(t, profile.NewProfile(profile.GlobalProfileID, "", "global prompt", "", nil, false))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	got, err := svc.Get(profile.DefaultProfileID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	assertProfile(t, got, profile.NewProfile(
		profile.DefaultProfileID,
		profile.DefaultProfileDescription,
		"global prompt\n\n"+prompt.DefaultAgentProfile(),
		profile.AnyModel,
		[]string{profile.FileSystem},
		profile.DefaultMemoryState,
	))
}

func TestGet_MergesGlobalPromptIntoSelectedProfile(t *testing.T) {
	storage := newTestStorage(t,
		newProfile("writer"),
		profile.NewProfile(profile.GlobalProfileID, "", "global prompt", "", nil, false),
	)
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	got, err := svc.Get("writer")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	assertProfile(t, got, profile.NewProfile(
		"writer",
		"description",
		"global prompt\n\nsystem prompt",
		"model-a",
		[]string{"filesystem"},
		true,
	))
}

func TestGet_PropagatesGlobalProfileError(t *testing.T) {
	wantErr := errors.New("load failed")
	storage := newMockRepo()
	storage.loadErr = wantErr
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if _, err := svc.Get(profile.DefaultProfileID); !errors.Is(err, wantErr) {
		t.Fatalf("Get() error = %v, want %v", err, wantErr)
	}
}

func TestSave_PersistsValidProfile(t *testing.T) {
	storage := newTestStorage(t)
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
	storage := newTestStorage(t)
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	p := profile.NewProfile("writer", "description", "system prompt", "ghost-model", []string{"filesystem"}, false)
	if err := svc.Save(p); !errors.Is(err, types.ErrIsNotExist) {
		t.Fatalf("Save() error = %v, want %v", err, types.ErrIsNotExist)
	}
	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no save, got %d", got)
	}
}

func TestSave_RejectsUnknownToolServer(t *testing.T) {
	storage := newTestStorage(t)
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	p := profile.NewProfile("writer", "description", "system prompt", "model-a", []string{"filesystem", "ghost"}, false)
	if err := svc.Save(p); !errors.Is(err, types.ErrIsNotExist) {
		t.Fatalf("Save() error = %v, want %v", err, types.ErrIsNotExist)
	}
	if got := storage.savedCount(); got != 0 {
		t.Fatalf("expected no save, got %d", got)
	}
}

func TestSave_SameIDOverwritesExistingProfile(t *testing.T) {
	storage := newTestStorage(t, newProfile("writer"))
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

func TestSave_PropagatesStorageError(t *testing.T) {
	wantErr := errors.New("save failed")
	storage := newMockRepo()
	storage.saveErr = wantErr
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if err := svc.Save(newProfile("writer")); !errors.Is(err, wantErr) {
		t.Fatalf("Save() error = %v, want %v", err, wantErr)
	}
}

func TestDelete_DeletesFromStorageAndSyncs(t *testing.T) {
	storage := newTestStorage(t, newProfile("writer"))
	sync1 := &mockSync{}
	sync2 := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))
	svc.AttachSyncer(sync1)
	svc.AttachSyncer(sync2)

	if err := svc.Delete("writer"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if got := storage.storedIDs(); len(got) != 0 {
		t.Fatalf("expected writer to be removed from storage, got %v", got)
	}
	if got := sync1.calledWith(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected sync1 delete of writer, got %v", got)
	}
	if got := sync2.calledWith(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected sync2 delete of writer, got %v", got)
	}
}

func TestDelete_UnknownProfileKeepsStorageIntact(t *testing.T) {
	storage := newTestStorage(t, newProfile("writer"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if err := svc.Delete("missing"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if got := storage.storedIDs(); !slices.Equal(got, []profile.ID{"writer"}) {
		t.Fatalf("expected storage to keep writer, got %v", got)
	}
}

func TestDelete_StorageErrorSkipsSyncs(t *testing.T) {
	wantErr := errors.New("save failed")
	storage := newTestStorage(t, newProfile("writer"))
	storage.saveErr = wantErr
	sync := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))
	svc.AttachSyncer(sync)

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
	storage := newTestStorage(t, newProfile("writer"))
	sync1 := &mockSync{err: syncErr}
	sync2 := &mockSync{}
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))
	svc.AttachSyncer(sync1)
	svc.AttachSyncer(sync2)

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
	storage := newTestStorage(t, newProfile("writer"))
	svc := newTestService(
		t,
		storage,
		newMockModelRegistry("model-a"),
		newMockToolRegistry("filesystem"),
	)
	svc.AttachSyncer(&mockSync{err: err1})
	svc.AttachSyncer(&mockSync{err: err2})

	err := svc.Delete("writer")
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Fatalf("Delete() error = %v, want both sync errors joined", err)
	}
}

func TestDelete_WithoutSyncs(t *testing.T) {
	storage := newTestStorage(t, newProfile("writer"))
	svc := newTestService(t, storage, newMockModelRegistry("model-a"), newMockToolRegistry("filesystem"))

	if err := svc.Delete("writer"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := storage.storedIDs(); len(got) != 0 {
		t.Fatalf("expected writer to be removed from storage, got %v", got)
	}
}
