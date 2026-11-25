package acceptance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/8thlight-daveloper/landline/testsupport/clock/sleeper"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	inviteAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	invalidCode    = "invalid or expired invite code"
)

var (
	joinCommandLine = regexp.MustCompile(`^landline server join (\S+) --code ([2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4})$`)
	twelveHex       = regexp.MustCompile(`\b[0-9a-f]{12}\b`)
	secretLine      = regexp.MustCompile(`(?m)^\s*secret\s*=`)
)

type hostRig struct {
	t      *testing.T
	bin    string
	home   string
	env    []string
	addr   string
	host   string
	port   string
	secret string
	url    string
	began  time.Time
	issued []string
}

func startHostWith(t *testing.T, flags ...string) *hostRig {
	t.Helper()
	return startHostWithEnv(t, nil, flags...)
}

func startHostWithEnv(t *testing.T, extra []string, flags ...string) *hostRig {
	t.Helper()
	bin, home, env := serverEnv(t)
	env = append(env, extra...)
	addr := freeAddr(t)
	began := time.Now()
	args := append([]string{"server", "up", "--addr", addr}, flags...)
	if out, err := runIn(t, bin, env, home, args...); err != nil {
		t.Fatalf("up: %v %s", err, safe(string(out)))
	}
	waitPastTheStartPeriod()
	host, port := hostAndPort(t, addr)
	values, _ := readTomlValues(t, configTomlOf(home))
	return &hostRig{t: t, bin: bin, home: home, env: env, addr: addr, host: host, port: port, secret: values["secret"], url: "http://" + addr, began: began}
}

func startHost(t *testing.T) *hostRig { return startHostWith(t) }

func (h *hostRig) run(args ...string) (string, error) {
	h.t.Helper()
	out, err := runIn(h.t, h.bin, h.env, h.home, args...)
	if err == nil && strings.Contains(strings.Join(args, " "), "rotate-secret") {
		values, _ := readTomlValues(h.t, configTomlOf(h.home))
		h.secret = values["secret"]
	}
	return out, err
}

func (h *hostRig) dataFile(name string) string {
	return filepath.Join(h.home, ".local", "share", "landline", name)
}

func (h *hostRig) inviteWithAddress(name string) (code, address string) {
	h.t.Helper()
	out, err := h.run("server", "invite", "--name", name)
	if err != nil {
		h.t.Fatalf("invite %s: %v %s", name, err, h.clean(out))
	}
	found := joinCommandLine.FindStringSubmatch(strings.TrimSpace(out))
	if found == nil {
		h.t.Fatalf("invite printed %q, want one line shaped like: landline server join <host:port> --code XXXX-XXXX", h.clean(out))
	}
	h.issued = append(h.issued, found[2])
	return found[2], found[1]
}

func (h *hostRig) invite(name string) string {
	h.t.Helper()
	code, _ := h.inviteWithAddress(name)
	return code
}

type machineRig struct {
	t    *testing.T
	host *hostRig
	home string
	env  []string
	name string
}

func (h *hostRig) newMachineHome() *machineRig {
	h.t.Helper()
	home := h.t.TempDir()
	return &machineRig{t: h.t, host: h, home: home, env: isolatedEnv(home)}
}

func (m *machineRig) run(args ...string) (string, error) {
	m.t.Helper()
	return runIn(m.t, m.host.bin, m.env, m.home, args...)
}

func (m *machineRig) joinWith(code string) (string, error) {
	m.t.Helper()
	return m.run("server", "join", m.host.addr, "--code", code)
}

func (h *hostRig) machine(name string) *machineRig {
	h.t.Helper()
	m := h.newMachineHome()
	m.name = name
	code, address := h.inviteWithAddress(name)
	if out, err := m.run("server", "join", address, "--code", code); err != nil {
		h.t.Fatalf("join %s through the printed address %s: %v %s", name, address, err, h.clean(out))
	}
	return m
}

func (m *machineRig) keyPath() string { return filepath.Join(configDirOf(m.home), "machine.key") }

func (m *machineRig) key() ed25519.PrivateKey {
	m.t.Helper()
	raw, err := os.ReadFile(m.keyPath())
	if err != nil {
		m.t.Fatal(err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		m.t.Fatalf("machine.key does not hold the base64 of a 32 byte seed: %v %d bytes", err, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed)
}

func (m *machineRig) publicKey() []byte { return m.key().Public().(ed25519.PublicKey) }

func (m *machineRig) agentToken(id string) string {
	m.t.Helper()
	raw, err := os.ReadFile(filepath.Join(configDirOf(m.home), "session-"+id+".json"))
	if err != nil {
		m.t.Fatal(err)
	}
	var session struct{ Token string }
	json.Unmarshal(raw, &session)
	return session.Token
}

func (m *machineRig) registerAgent(id string) {
	m.t.Helper()
	if _, err := m.run("register", "--id", id, "--harness", "claude-code", "--host", "mac"); err != nil {
		m.t.Fatalf("register %s on %s: %v", id, m.name, err)
	}
}

func listedMachines(t *testing.T, h *hostRig) string {
	t.Helper()
	out, err := h.run("server", "machines")
	if err != nil {
		t.Fatalf("machines: %v %s", err, safe(string(out)))
	}
	return out
}

func randomCode(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	chars := make([]byte, 8)
	for i, b := range raw {
		chars[i] = inviteAlphabet[int(b)%len(inviteAlphabet)]
	}
	return string(chars[:4]) + "-" + string(chars[4:])
}

func signedText(name string, unix int64, nonce, method, path string, body []byte) []string {
	sum := sha256.Sum256(body)
	return []string{"landline-machine-v1", name, strconv.FormatInt(unix, 10), nonce, method, path, hex.EncodeToString(sum[:])}
}

func machineSignature(priv ed25519.PrivateKey, name string, unix int64, nonce, method, path string, body []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(strings.Join(signedText(name, unix, nonce, method, path, body), "\n"))))
}

func machineHeader(name string, unix int64, nonce, signature string) string {
	return fmt.Sprintf("Landline-Machine name=%s, time=%d, nonce=%s, signature=%s", name, unix, nonce, signature)
}

func freshNonce() string {
	raw := make([]byte, 16)
	rand.Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}

func machineAuthWith(priv ed25519.PrivateKey, name string, unix int64, nonce, method, path string, body []byte) string {
	return machineHeader(name, unix, nonce, machineSignature(priv, name, unix, nonce, method, path, body))
}

func machineAuth(priv ed25519.PrivateKey, name string, unix int64, method, path string, body []byte) string {
	return machineAuthWith(priv, name, unix, freshNonce(), method, path, body)
}

func (h *hostRig) signed(m *machineRig, unix int64, method, path string, body []byte) (int, string) {
	h.t.Helper()
	return h.rawRequest(method, path, body, map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, method, path, body)})
}

func registerBody(id string) []byte {
	raw, _ := json.Marshal(map[string]string{"id": id, "harness": "claude-code", "host": "mac"})
	return raw
}

func filesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	found := map[string]string{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			raw, _ := os.ReadFile(path)
			found[path] = string(raw)
		}
		return nil
	})
	return found
}

func TestInviteStartsAJoinCommandWithOneCodeInTheDocumentedShapeAndNoSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	out, err := h.run("server", "invite", "--name", "linux-box")
	if err != nil {
		t.Fatalf("invite: %v %s", err, h.clean(out))
	}
	if strings.Count(strings.TrimSpace(out), "\n") != 0 {
		t.Errorf("invite printed more than one line: %q", h.clean(out))
	}
	found := joinCommandLine.FindStringSubmatch(strings.TrimSpace(out))
	if found == nil {
		t.Fatalf("invite printed %q", h.clean(out))
	}
	host, port, err := net.SplitHostPort(found[1])
	if err != nil || port != h.port || host == "" || host == "0.0.0.0" || host == "::" {
		t.Errorf("the join command names %q, want a usable host and the saved port %s", found[1], h.port)
	}
	if strings.Contains(out, h.secret) {
		t.Error("invite printed the shared secret")
	}
}

func TestInviteRefusesANameThatIsNotOneToThirtyTwoLowerCaseLettersDigitsAndHyphensStartingWithALetter(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.invite("valid-name")
	for _, name := range []string{"", "Linux", "1box", "-box", "has space", "under_score", "box.local", strings.Repeat("a", 33)} {
		out, err := h.run("server", "invite", "--name", name)
		if err == nil || joinCommandLine.MatchString(strings.TrimSpace(out)) || regexp.MustCompile(`[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}`).MatchString(out) {
			t.Errorf("name %q: %v %q", name, err, h.clean(out))
		}
	}
	if out, err := h.run("server", "invite"); err == nil || joinCommandLine.MatchString(strings.TrimSpace(out)) {
		t.Errorf("no name: %v %q", err, safe(string(out)))
	}
}

func TestInviteAcceptsAOneLetterNameAThirtyTwoCharacterNameAndHyphensAndDigits(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	for _, name := range []string{"a", strings.Repeat("a", 32), "box-1", "a1-b2-c3", "a-", "a--b"} {
		h.invite(name)
	}
}

func TestInviteRefusesANameThatAlreadyBelongsToAMachineWithA409AndSaysToRevokeIt(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	out, err := h.run("server", "invite", "--name", "linux-box")
	if err == nil || !strings.Contains(out, "landline server revoke") || joinCommandLine.MatchString(strings.TrimSpace(out)) {
		t.Fatalf("invite for a taken name: %v %q", err, h.clean(out))
	}
	if status, _ := h.adminStatus("POST", "/v1/machines", []byte(`{"name":"linux-box"}`)); status != http.StatusConflict {
		t.Errorf("POST /v1/machines for a taken name answered %d, want 409", status)
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the refused invite removed the machine")
	}
}

func TestAnInviteForAPendingNameIsRefusedUntilThePendingInviteIsRevoked(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	first := h.invite("linux-box")
	out, err := h.run("server", "invite", "--name", "linux-box")
	if err == nil || !strings.Contains(out, "revoke") || joinCommandLine.MatchString(strings.TrimSpace(out)) {
		t.Fatalf("a second invite for a pending name: %v %q", err, h.clean(out))
	}
	if status, _ := h.adminStatus("POST", "/v1/machines", []byte(`{"name":"linux-box"}`)); status != http.StatusConflict {
		t.Errorf("POST /v1/machines for a pending name answered %d, want 409", status)
	}
	m := h.newMachineHome()
	if out, err := m.joinWith(first); err != nil {
		t.Fatalf("the first code stopped working after a refused invite: %v %s", err, h.clean(out))
	}
	pending := h.invite("pending-box")
	if out, err := h.run("server", "revoke", "pending-box"); err != nil {
		t.Fatalf("revoke of a pending invite: %v %s", err, safe(string(out)))
	}
	if out, err := h.newMachineHome().joinWith(pending); err == nil || !strings.Contains(out, invalidCode) {
		t.Errorf("the code of a revoked pending invite: %v %q", err, h.clean(out))
	}
	h.invite("pending-box")
}

func TestEveryAdministrationCommandRefusesWhenTheSavedHostIsNotThisMachinesAddress(t *testing.T) {
	t.Parallel()
	control := startHost(t)
	control.invite("control-box")
	listedMachines(t, control)
	bin, home, env := serverEnv(t)
	writeConfigFile(t, home, "config.toml", tomlFor(unresolvableHost, "8787", joinedSecret+joinedSecret))
	before, _ := os.ReadFile(configTomlOf(home))
	for _, args := range [][]string{{"server", "invite", "--name", "linux-box"}, {"server", "machines"}, {"server", "revoke", "linux-box"}, {"server", "revoke", "--all"}, {"server", "rotate-secret"}} {
		out, err := runIn(t, bin, env, home, args...)
		lower := strings.ToLower(out)
		if exitCodeOf(err) != 1 || joinCommandLine.MatchString(strings.TrimSpace(out)) || !strings.Contains(out, unresolvableHost) ||
			strings.Contains(lower, "lookup") || strings.Contains(lower, "dial") || strings.Contains(lower, "no such host") {
			t.Errorf("%v: exit %d, want 1 with a refusal that names the saved host and is not a failed connection: %q", args, exitCodeOf(err), safe(out))
		}
	}
	if after, _ := os.ReadFile(configTomlOf(home)); string(after) != string(before) {
		t.Errorf("config.toml changed:\n%s", scrub(string(after)))
	}
}

func TestInviteMachinesAndRevokeFailWhenTheSharedSecretIsWrong(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	wrong := append(append([]string{}, h.env...), "LANDLINE_SECRET="+strings.Repeat("x", 32))
	for _, args := range [][]string{{"server", "invite", "--name", "other"}, {"server", "machines"}, {"server", "revoke", "linux-box"}} {
		out, err := runIn(t, h.bin, wrong, h.home, args...)
		if err == nil || joinCommandLine.MatchString(strings.TrimSpace(out)) {
			t.Errorf("%v with a wrong secret: %v %q", args, err, h.clean(out))
		}
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("a wrong secret revoked the machine")
	}
}

func TestAdminCommandsKeepUsingTheSharedSecretWhenAKeyFileIsAlsoPresentOnTheSwitchboardMachine(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	raw, _ := os.ReadFile(m.keyPath())
	if err := os.WriteFile(filepath.Join(configDirOf(h.home), "machine.key"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	configRaw := readFileOrFail(t, configTomlOf(h.home))
	if err := os.WriteFile(configTomlOf(h.home), []byte(configRaw+"machine = \"linux-box\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("machines failed or lost the machine when a key file sat beside the secret")
	}
	h.invite("other-box")
}

func TestAMachineThatHoldsOnlyAKeyCannotInviteListOrRevokeAndTheSwitchboardIsUnchanged(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	for _, args := range [][]string{{"server", "invite", "--name", "other-box"}, {"server", "machines"}, {"server", "revoke", "linux-box"}, {"server", "revoke", "--all"}} {
		out, err := m.run(args...)
		if err == nil || joinCommandLine.MatchString(strings.TrimSpace(out)) || !strings.Contains(out, noCredential) {
			t.Errorf("%v from a machine with only a key: %v %q", args, err, safe(string(out)))
		}
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("a machine with only a key revoked itself")
	}
}

func TestJoinWithACodeSavesA32ByteSeedInAnOwnerOnlyFileAndTheAddressAndNoSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	info, err := os.Stat(m.keyPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("machine.key %v %v", info, err)
	}
	raw, _ := os.ReadFile(m.keyPath())
	if strings.Count(strings.TrimRight(string(raw), "\n"), "\n") != 0 {
		t.Errorf("machine.key holds more than one line")
	}
	m.key()
	if info, err := os.Stat(configTomlOf(m.home)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config.toml %v %v", info, err)
	}
	values, lines := readTomlValues(t, configTomlOf(m.home))
	if values["host"] == "" || values["port"] != h.port || values["machine"] != "linux-box" {
		t.Errorf("config.toml holds %v, want host %s, port %s, and machine linux-box", redacted(values), h.host, h.port)
	}
	if _, has := values["secret"]; has {
		t.Errorf("config.toml still holds a secret:\n%s", scrub(strings.Join(lines, "\n")))
	}
	if secretLine.MatchString(strings.Join(lines, "\n")) {
		t.Error("config.toml still has a secret line")
	}
}

func TestJoinWithACodeRemovesOnlyTheSecretLineAndKeepsCommentsAndUnknownKeys(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.newMachineHome()
	writeConfigFile(t, m.home, "config.toml", "# my notes\nsecret = \""+joinedSecret+joinedSecret+"\"\nhost = \"old.invalid\"\nport = 1\nnickname = \"keep me\"\n")
	if out, err := m.joinWith(h.invite("linux-box")); err != nil {
		t.Fatalf("join: %v %s", err, safe(string(out)))
	}
	raw, _ := os.ReadFile(configTomlOf(m.home))
	text := string(raw)
	if !strings.Contains(text, "# my notes") || !strings.Contains(text, `nickname = "keep me"`) || secretLine.MatchString(text) || strings.Contains(text, joinedSecret) {
		t.Errorf("config.toml after join:\n%s", scrub(text))
	}
	values, _ := readTomlValues(t, configTomlOf(m.home))
	if values["host"] != h.host || values["port"] != h.port {
		t.Errorf("host and port: %v", redacted(values))
	}
}

func TestJoinWithACodePutsNeitherTheSharedSecretNorAnotherMachinesKeyOnTheNewMachine(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	other := h.machine("other-box")
	otherSeed := strings.TrimSpace(readFileOrFail(t, other.keyPath()))
	m := h.newMachineHome()
	out, err := m.joinWith(h.invite("linux-box"))
	if err != nil {
		t.Fatalf("join: %v %s", err, h.clean(out))
	}
	ownSeed := strings.TrimSpace(readFileOrFail(t, m.keyPath()))
	if strings.Contains(out, h.secret) || strings.Contains(out, otherSeed) || strings.Contains(out, ownSeed) {
		t.Error("join printed the shared secret or a machine's private key")
	}
	for path, content := range filesUnder(t, m.home) {
		if strings.Contains(content, h.secret) || strings.Contains(content, otherSeed) {
			t.Errorf("%s holds the shared secret or another machine's key", path)
		}
	}
}

func readFileOrFail(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestACodeWorksOnceAndTheSecondUseGetsTheSameMessageAsAWrongCode(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	first := h.newMachineHome()
	if out, err := first.joinWith(code); err != nil {
		t.Fatalf("first use: %v %s", err, h.clean(out))
	}
	second, third := h.newMachineHome(), h.newMachineHome()
	wrongCode := randomCode(t)
	reused, reusedErr := second.joinWith(code)
	wrong, wrongErr := third.joinWith(wrongCode)
	if reusedErr == nil || wrongErr == nil || !strings.Contains(reused, invalidCode) || !strings.Contains(wrong, invalidCode) {
		t.Fatalf("reused: %v %q, wrong: %v %q", reusedErr, h.clean(reused), wrongErr, h.clean(wrong))
	}
	if strings.ReplaceAll(strings.TrimSpace(reused), code, "") != strings.ReplaceAll(strings.TrimSpace(wrong), wrongCode, "") {
		t.Errorf("a used code and a wrong code are told apart:\n%q\n%q", h.clean(reused), h.clean(wrong))
	}
	if _, statErr := os.Stat(second.keyPath()); statErr == nil {
		t.Error("a used code left a key on the second machine")
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the first machine is gone")
	}
}

func TestAWrongCodeSaysInvalidOrExpiredAndChangesNoFileOnTheMachineNotEvenAnExistingKey(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.invite("linux-box")
	existing := h.machine("existing-box")
	keyBefore := readFileOrFail(t, existing.keyPath())
	configBefore := readFileOrFail(t, configTomlOf(existing.home))
	out, err := existing.joinWith(randomCode(t))
	if err == nil || !strings.Contains(out, invalidCode) {
		t.Fatalf("join with a wrong code: %v %q", err, h.clean(out))
	}
	if readFileOrFail(t, existing.keyPath()) != keyBefore {
		t.Error("a failed join changed or removed the machine's working key")
	}
	if readFileOrFail(t, configTomlOf(existing.home)) != configBefore {
		t.Error("a failed join changed config.toml")
	}
	fresh := h.newMachineHome()
	seeded := "# my notes\nsecret = \"" + joinedSecret + joinedSecret + "\"\nhost = \"old.invalid\"\nport = 1\nnickname = \"keep me\"\n"
	writeConfigFile(t, fresh.home, "config.toml", seeded)
	if out, err := fresh.joinWith(randomCode(t)); err == nil || !strings.Contains(out, invalidCode) {
		t.Fatalf("join with a wrong code on a new machine: %v %q", err, h.clean(out))
	}
	if _, statErr := os.Stat(fresh.keyPath()); statErr == nil {
		t.Error("machine.key was written")
	}
	if readFileOrFail(t, configTomlOf(fresh.home)) != seeded {
		t.Error("a failed join changed a config.toml that held a secret")
	}
	if strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("a wrong code made a machine")
	}
}

func TestAJoinToASwitchboardThatIsNotThereFailsAndChangesNoFile(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("control-box")
	m := h.newMachineHome()
	seeded := "secret = \"" + joinedSecret + joinedSecret + "\"\nhost = \"old.invalid\"\nport = 1\n"
	writeConfigFile(t, m.home, "config.toml", seeded)
	out, err := m.run("server", "join", net.JoinHostPort("127.0.0.1", freePort(t)), "--code", randomCode(t))
	if err == nil {
		t.Fatalf("join to a closed port succeeded: %s", safe(string(out)))
	}
	if readFileOrFail(t, configTomlOf(m.home)) != seeded {
		t.Error("a failed join changed config.toml")
	}
	if _, statErr := os.Stat(m.keyPath()); statErr == nil {
		t.Error("machine.key was written")
	}
}

func failRedeems(t *testing.T, h *hostRig, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if out, err := h.newMachineHome().joinWith(randomCode(t)); err == nil || !strings.Contains(out, invalidCode) {
			t.Fatalf("failed redeem %d: %v %q", i+1, err, h.clean(out))
		}
	}
}

func TestFourFailedRedeemsLeaveAnInviteUsableAndAGoodRedeemStartsTheCountAgain(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	for _, name := range []string{"first-box", "second-box"} {
		code := h.invite(name)
		failRedeems(t, h, 4)
		if out, err := h.newMachineHome().joinWith(code); err != nil {
			t.Fatalf("%s: four failures cancelled the invite: %v %s", name, err, h.clean(out))
		}
	}
}

func TestFiveFailedRedeemsCancelEveryWaitingInviteEvenWhenAnInviteWasMadeBetweenThem(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	first := h.invite("first-box")
	failRedeems(t, h, 3)
	second := h.invite("second-box")
	failRedeems(t, h, 2)
	for _, code := range []string{first, second} {
		if out, err := h.newMachineHome().joinWith(code); err == nil || !strings.Contains(out, invalidCode) {
			t.Errorf("a waiting invite survived five failed redeems: %v %q", err, h.clean(out))
		}
	}
	h.invite("fresh-box")
}

func TestMalformedRedeemRequestsCountAsFailuresLikeAnyOther(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	bodies := [][]byte{[]byte(``), []byte(`not json`), []byte(`{}`), []byte(`{"code":null}`), []byte(`{"code":12345678}`)}
	code := h.invite("linux-box")
	for _, body := range bodies[:4] {
		h.statusOnly("PATCH", "/v1/machines", body, nil)
	}
	if out, err := h.newMachineHome().joinWith(code); err != nil {
		t.Fatalf("four malformed requests cancelled the invite: %v %s", err, h.clean(out))
	}
	code = h.invite("second-box")
	for _, body := range bodies {
		h.statusOnly("PATCH", "/v1/machines", body, nil)
	}
	if out, err := h.newMachineHome().joinWith(code); err == nil || !strings.Contains(out, invalidCode) {
		t.Errorf("five malformed requests left the invite usable: %v %q", err, h.clean(out))
	}
}

func TestACodeIsReadWithoutTheHyphenAndInAnyCase(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	for name, spell := range map[string]func(string) string{
		"lower-box":   strings.ToLower,
		"bare-box":    func(code string) string { return strings.ReplaceAll(code, "-", "") },
		"lowbare-box": func(code string) string { return strings.ToLower(strings.ReplaceAll(code, "-", "")) },
	} {
		code := spell(h.invite(name))
		if out, err := h.newMachineHome().joinWith(code); err != nil {
			t.Errorf("%s: the code spelled as typed did not work: %v %s", name, err, h.clean(out))
		}
	}
}

func TestRestartingTheSwitchboardCancelsWaitingInvites(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	h.restartAndSettle()
	if out, err := h.newMachineHome().joinWith(code); err == nil || !strings.Contains(out, invalidCode) {
		t.Fatalf("an invite survived a restart: %v %q", err, h.clean(out))
	}
}

func TestJoinWithACodeReplacesAnExistingKeyAndTheOldNameStaysListedUntilRevoked(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("old-name")
	oldSeed, _ := os.ReadFile(m.keyPath())
	if out, err := m.joinWith(h.invite("new-name")); err != nil {
		t.Fatalf("second join: %v %s", err, safe(string(out)))
	}
	if info, err := os.Stat(m.keyPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("machine.key after a replacing join %v %v", info, err)
	}
	if values, _ := readTomlValues(t, configTomlOf(m.home)); values["machine"] != "new-name" {
		t.Errorf("machine after a second join is %q, want new-name", values["machine"])
	}
	m.name = "new-name"
	m.registerAgent("alpha")
	if newSeed, _ := os.ReadFile(m.keyPath()); string(newSeed) == string(oldSeed) {
		t.Error("the key file kept the old key")
	}
	listed := listedMachines(t, h)
	if !strings.Contains(listed, "old-name") || !strings.Contains(listed, "new-name") {
		t.Errorf("machines after a second join:\n%s", listed)
	}
}

func TestRedeemOverRawHTTPNeedsNoAuthorizationAnswersTheNameAndSucceedsOnlyOnceForARightCode(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	wrong, _ := json.Marshal(map[string]string{"code": randomCode(t), "public_key": public})
	if status, body := h.rawRequest("PATCH", "/v1/machines", wrong, nil); status != http.StatusUnauthorized || !strings.Contains(body, invalidCode) {
		t.Fatalf("a wrong code: %d %s", status, h.clean(body))
	}
	right, _ := json.Marshal(map[string]string{"code": strings.ToLower(strings.ReplaceAll(code, "-", "")), "public_key": public})
	status, body := h.rawRequest("PATCH", "/v1/machines", right, nil)
	var answer struct{ Name string }
	if status < 200 || status > 299 || json.Unmarshal([]byte(body), &answer) != nil || answer.Name != "linux-box" {
		t.Fatalf("the right code answered %d with a body that does not name the machine", status)
	}
	if status, body := h.rawRequest("PATCH", "/v1/machines", right, nil); status != http.StatusUnauthorized || !strings.Contains(body, invalidCode) {
		t.Errorf("the same code a second time: %d %s", status, h.clean(body))
	}
	unix := time.Now().Unix()
	headers := map[string]string{"Authorization": machineAuth(priv, "linux-box", unix, "POST", "/v1/register", registerBody("alpha"))}
	if status := h.statusOnly("POST", "/v1/register", registerBody("alpha"), headers); status != http.StatusOK {
		t.Fatalf("a registration signed with the key that redeemed answered %d", status)
	}
}

func TestEveryRedeemThatDoesNotNameAValidPendingCodeGetsTheSame401AndTheSameMessage(t *testing.T) {
	t.Parallel()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	cases := map[string]string{
		"no body":             ``,
		"not json":            `not json`,
		"an empty object":     `{}`,
		"no code":             `{"public_key":"` + public + `"}`,
		"a null code":         `{"code":null,"public_key":"` + public + `"}`,
		"an empty code":       `{"code":"","public_key":"` + public + `"}`,
		"a number":            `{"code":12345678,"public_key":"` + public + `"}`,
		"a list":              `{"code":["ABCD-EFGH"],"public_key":"` + public + `"}`,
		"a short code":        `{"code":"ABCD-EFG","public_key":"` + public + `"}`,
		"a long code":         `{"code":"ABCD-EFGHJ","public_key":"` + public + `"}`,
		"a letter not in use": `{"code":"ABCD-EFGI","public_key":"` + public + `"}`,
		"a zero":              `{"code":"ABCD-EF0H","public_key":"` + public + `"}`,
		"an unknown code":     `{"code":"WWWW-WWWW","public_key":"` + public + `"}`,
	}
	var mu sync.Mutex
	bodies := map[string]string{}
	for label, payload := range cases {
		label, payload := label, payload
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			h := startHost(t)
			h.invite("linux-box")
			status, body := h.rawRequest("PATCH", "/v1/machines", []byte(payload), nil)
			if status != http.StatusUnauthorized || !strings.Contains(body, invalidCode) {
				t.Errorf("%s: %d %s", label, status, h.clean(body))
			}
			mu.Lock()
			bodies[label] = strings.TrimSpace(body)
			mu.Unlock()
		})
	}
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		var first string
		for label, body := range bodies {
			if first == "" {
				first = body
			}
			if body != first {
				t.Errorf("the 401 for %q differs from the others", label)
			}
		}
	})
}

func TestAUsedOrRevokedCodeGetsTheSame401AsAWrongCodeOverRawHTTP(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	body := func(code string) []byte {
		raw, _ := json.Marshal(map[string]string{"code": code, "public_key": base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))})
		return raw
	}
	used, revoked := h.invite("used-box"), h.invite("revoked-box")
	if status := h.statusOnly("PATCH", "/v1/machines", body(used), nil); status < 200 || status > 299 {
		t.Fatalf("the first use answered %d", status)
	}
	h.run("server", "revoke", "revoked-box")
	var seen []string
	for label, code := range map[string]string{"used": used, "revoked": revoked, "wrong": randomCode(t)} {
		status, answer := h.rawRequest("PATCH", "/v1/machines", body(code), nil)
		if status != http.StatusUnauthorized || !strings.Contains(answer, invalidCode) {
			t.Errorf("%s code: %d", label, status)
		}
		seen = append(seen, strings.TrimSpace(answer))
	}
	for _, each := range seen {
		if each != seen[0] {
			t.Error("the answers for a used, revoked, and wrong code differ")
		}
	}
}

func TestAnExpiredCodeGetsTheSame401AsAWrongCodeOverRawHTTP(t *testing.T) {
	t.Parallel()
	h := startHostWith(t, "--invite-lifetime", "3s")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	body := func(code string) []byte {
		raw, _ := json.Marshal(map[string]string{"code": code, "public_key": base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))})
		return raw
	}
	expired := h.invite("expired-box")
	time.Sleep(4 * time.Second)
	expiredStatus, expiredAnswer := h.rawRequest("PATCH", "/v1/machines", body(expired), nil)
	status, wrongAnswer := h.rawRequest("PATCH", "/v1/machines", body(randomCode(t)), nil)
	if expiredStatus != http.StatusUnauthorized || status != http.StatusUnauthorized || !strings.Contains(expiredAnswer, invalidCode) || strings.TrimSpace(expiredAnswer) != strings.TrimSpace(wrongAnswer) {
		t.Errorf("an expired code and a wrong code are told apart")
	}
}

func TestAValidCodeWithAMissingOrMalformedKeyAnswers400AndIsNeitherUsedUpNorCounted(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	failRedeems(t, h, 4)
	for label, key := range map[string]any{"missing": nil, "empty": "", "not base64": "this is not base64!!", "31 bytes": base64.StdEncoding.EncodeToString(make([]byte, 31)), "33 bytes": base64.StdEncoding.EncodeToString(make([]byte, 33)), "a number": 5} {
		payload := map[string]any{"code": code}
		if key != nil {
			payload["public_key"] = key
		}
		raw, _ := json.Marshal(payload)
		if status := h.statusOnly("PATCH", "/v1/machines", raw, map[string]string{"Content-Type": "application/json"}); status != http.StatusBadRequest {
			t.Errorf("%s key answered %d, want 400", label, status)
		}
	}
	if out, err := h.newMachineHome().joinWith(code); err != nil {
		t.Errorf("after four failures and six bad keys the code no longer works: %v %s", err, h.clean(out))
	}
	if out, err := h.run("server", "machines"); err != nil {
		t.Errorf("machines: %v %s", err, safe(string(out)))
	}
}

func TestTwoRedeemsOfOneCodeAtTheSameTimeAdmitExactlyOne(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	var wait sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for i := 0; i < 8; i++ {
		_, priv, _ := ed25519.GenerateKey(rand.Reader)
		body, _ := json.Marshal(map[string]string{"code": code, "public_key": base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))})
		wait.Add(1)
		go func() {
			defer wait.Done()
			if status := h.statusQuiet("PATCH", "/v1/machines", body, map[string]string{"Content-Type": "application/json"}); status >= 200 && status <= 299 {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wait.Wait()
	if admitted != 1 {
		t.Errorf("%d of 8 simultaneous redeems of one code were admitted, want exactly 1", admitted)
	}
}

func storedEntries(t *testing.T, text string) []map[string]any {
	t.Helper()
	var root any
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		t.Fatalf("machines.json is not JSON: %v", err)
	}
	var found []map[string]any
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if _, ok := value["public_key"]; ok {
				found = append(found, value)
			}
			for _, each := range value {
				walk(each)
			}
		case []any:
			for _, each := range value {
				walk(each)
			}
		}
	}
	walk(root)
	return found
}

func TestMachinesJsonSitsNextToTheLogIsOwnerOnlyAndHoldsTheNameTheKeyAndAUTCTimeInTheNamedFieldsButNoSeedCodeOrSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	m := h.newMachineHome()
	joined := time.Now()
	if out, err := m.joinWith(code); err != nil {
		t.Fatalf("join: %v %s", err, h.clean(out))
	}
	m.name = "linux-box"
	info, err := os.Stat(h.dataFile("machines.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("machines.json %v %v", info, err)
	}
	text := readFileOrFail(t, h.dataFile("machines.json"))
	entries := storedEntries(t, text)
	if len(entries) != 1 {
		t.Fatalf("machines.json holds %d entries with a public_key field, want 1", len(entries))
	}
	entry := entries[0]
	if entry["name"] != "linux-box" || entry["public_key"] != base64.StdEncoding.EncodeToString(m.publicKey()) {
		t.Error("the entry's name or public_key field does not match the machine")
	}
	created, _ := entry["created"].(string)
	when, parseErr := time.Parse(time.RFC3339, created)
	if parseErr != nil || !strings.HasSuffix(created, "Z") || when.Sub(joined) < -time.Minute || when.Sub(joined) > time.Minute {
		t.Errorf("the created field is not an RFC 3339 UTC time near the join: %v", parseErr)
	}
	seed := strings.TrimSpace(readFileOrFail(t, m.keyPath()))
	if strings.Contains(text, seed) {
		t.Error("machines.json holds a machine's private seed")
	}
	if strings.Contains(text, code) || strings.Contains(text, strings.ReplaceAll(code, "-", "")) {
		t.Error("machines.json holds an invite code")
	}
	if strings.Contains(text, h.secret) {
		t.Error("machines.json holds the shared secret")
	}
	if out, err := h.run("server", "revoke", "--all"); err != nil {
		t.Fatalf("revoke --all: %v %s", err, safe(string(out)))
	}
	if info, err := os.Stat(h.dataFile("machines.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("machines.json after a revoke %v %v", info, err)
	}
}

func TestNoSwitchboardFileHoldsAnInviteCodeInEitherSpellingATriedCodeAKeySeedOrTheSharedSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	pending := h.invite("pending-box")
	tried := randomCode(t)
	m := h.newMachineHome()
	m.joinWith(code)
	h.newMachineHome().joinWith(tried)
	m.name = "linux-box"
	m.registerAgent("alpha")
	seed := strings.TrimSpace(readFileOrFail(t, m.keyPath()))
	needles := map[string]string{"an invite code": code, "an invite code without its hyphen": strings.ReplaceAll(code, "-", ""), "a code that was tried": tried, "a pending invite's code": pending, "a pending invite's code without its hyphen": strings.ReplaceAll(pending, "-", ""), "a machine's seed": seed, "the shared secret": h.secret}
	for path, content := range filesUnder(t, filepath.Join(h.home, ".local", "share", "landline")) {
		for what, needle := range needles {
			if strings.Contains(content, needle) {
				t.Errorf("%s holds %s", filepath.Base(path), what)
			}
		}
	}
}

func TestMachinesSurviveARestartOfTheSwitchboardAndKeepWorking(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	h.restartAndSettle()
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the machine did not survive a restart")
	}
	m.registerAgent("bravo")
	if out, err := m.run("agents", "--json"); err != nil || !strings.Contains(out, "bravo") {
		t.Errorf("agents after the restart: %v %s", err, safe(string(out)))
	}
}

func TestAMachineWithOnlyAKeyRegistersListsAgentsAndReadsTheLogWithNoSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	if values, _ := readTomlValues(t, configTomlOf(m.home)); values["secret"] != "" {
		t.Fatalf("the machine holds a secret: %v", redacted(values))
	}
	m.registerAgent("alpha")
	if out, err := m.run("agents", "--json"); err != nil || !strings.Contains(out, "alpha") {
		t.Errorf("agents: %v %s", err, safe(string(out)))
	}
	if out, err := m.run("log"); err != nil {
		t.Errorf("log: %v %s", err, safe(string(out)))
	}
}

func TestAnMCPProcessOnAMachineWithOnlyAKeyRegistersAndListsItselfAmongTheAgents(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	cmd := exec.Command(mcpOf(h.bin))
	cmd.Dir, cmd.Env = m.home, m.env
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	if got := idInRegisterResult(t, p.tool("landline_register", `{"project":"remote-proj"}`)); got == "" {
		t.Fatal("no id")
	}
	if got := p.tool("landline_agents", `{}`); !strings.Contains(got, "project: remote-proj") {
		t.Errorf("the agent list does not show the registered session: %s", got)
	}
}

func TestTheMenuBarOnAMachineWithOnlyAKeyReachesTheSwitchboard(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	cmd := exec.Command(menubarBinary(t, defaultFlags(t)), "menu", "--text")
	cmd.Dir, cmd.Env = m.home, m.env
	out, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(out), "Switchboard unreachable") || !strings.Contains(string(out), "alpha") {
		t.Fatalf("menubar --text: %v\n%s", err, safe(string(out)))
	}
}

func TestTheEnvironmentSecretComesFirstThenTheKeyThenTheSecretInTheFileForTheCommandsTheScreenTheMCPProcessAndTheMenuBar(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	wrong := append(append([]string{}, m.env...), "LANDLINE_SECRET="+strings.Repeat("x", 32))
	if out, err := runIn(t, h.bin, wrong, m.home, "agents", "--json"); err == nil {
		t.Errorf("a wrong LANDLINE_SECRET did not win over the key: %s", safe(string(out)))
	}
	for _, args := range [][]string{{"register", "--id", "envbox", "--harness", "claude-code", "--host", "mac"}, {"log"}} {
		if out, err := runIn(t, h.bin, wrong, m.home, args...); err == nil {
			t.Errorf("%v: a wrong LANDLINE_SECRET did not win over the key: %s", args, safe(out))
		}
	}
	mcpWrong := exec.Command(mcpOf(h.bin))
	mcpWrong.Dir, mcpWrong.Env = m.home, wrong
	pw := newMCP(t, mcpWrong)
	pw.handshake("claude-code", "2.1.296")
	if got := pw.tool("landline_register", `{"project":"env-proj"}`); strings.Contains(got, "you are") {
		t.Errorf("the MCP process: a wrong LANDLINE_SECRET did not win over the key: %s", safe(got))
	}
	menuWrong := exec.Command(menubarBinary(t, defaultFlags(t)), "menu", "--text")
	menuWrong.Dir, menuWrong.Env = m.home, wrong
	if out, _ := menuWrong.CombinedOutput(); !strings.Contains(string(out), "Switchboard unreachable") {
		t.Errorf("the menu bar: a wrong LANDLINE_SECRET did not win over the key: %s", safe(string(out)))
	}
	right := append(append([]string{}, m.env...), "LANDLINE_SECRET="+h.secret)
	if out, err := runIn(t, h.bin, right, m.home, "agents", "--json"); err != nil {
		t.Errorf("the right LANDLINE_SECRET did not work: %v %s", err, safe(string(out)))
	}
	raw := readFileOrFail(t, configTomlOf(m.home))
	if err := os.WriteFile(configTomlOf(m.home), []byte(raw+"secret = \""+strings.Repeat("y", 32)+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := m.run("agents", "--json"); err != nil {
		t.Errorf("the screen reader: a stale secret in the file beat a working key: %v %s", err, safe(string(out)))
	}
	if out, err := m.run("log"); err != nil {
		t.Errorf("the command reader: a stale secret in the file beat a working key: %v %s", err, safe(string(out)))
	}
	cmd := exec.Command(mcpOf(h.bin))
	cmd.Dir, cmd.Env = m.home, m.env
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	p.tool("landline_register", `{"project":"order-proj"}`)
	if got := p.tool("landline_agents", `{}`); !strings.Contains(got, "project: order-proj") {
		t.Errorf("the MCP process: a stale secret in the file beat a working key: %s", got)
	}
	m.registerAgent("alpha")
	menu := exec.Command(menubarBinary(t, defaultFlags(t)), "menu", "--text")
	menu.Dir, menu.Env = m.home, m.env
	if out, err := menu.CombinedOutput(); err != nil || strings.Contains(string(out), "Switchboard unreachable") || !strings.Contains(string(out), "alpha") {
		t.Errorf("the menu bar: a stale secret in the file beat a working key: %v\n%s", err, safe(string(out)))
	}
}

const noCredential = "there is no switchboard credential"

func TestEveryClientWithAHostAndNoSecretAndNoKeyPrintsTheSameMessageThatNamesJoinAndTheSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.newMachineHome()
	writeConfigFile(t, m.home, "config.toml", tomlFor(h.host, h.port, ""))
	for _, args := range [][]string{{"agents", "--json"}, {"log"}, {"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"}} {
		out, err := m.run(args...)
		if err == nil {
			t.Errorf("%v succeeded with no credential: %s", args, safe(string(out)))
		}
		for _, want := range []string{noCredential, "landline server join", "LANDLINE_SECRET"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: the message does not hold %q: %q", args, want, safe(string(out)))
			}
		}
	}
	cmd := exec.Command(mcpOf(h.bin))
	cmd.Dir, cmd.Env = m.home, m.env
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	got := p.tool("landline_agents", `{}`)
	for _, want := range []string{noCredential, "landline server join", "LANDLINE_SECRET"} {
		if !strings.Contains(got, want) {
			t.Errorf("the MCP process: the message does not hold %q: %q", want, got)
		}
	}
}

func TestAKeyFileThatIsNotAKeyIsAnErrorThatNamesTheFileAndNeverRepeatsItsContent(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	for name, content := range map[string]string{
		"not base64":   "this-is-not-base64-content-hunter2!!\n",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("too short hunter2")) + "\n",
		"empty":        "\n",
	} {
		m := h.newMachineHome()
		writeConfigFile(t, m.home, "config.toml", tomlFor(h.host, h.port, ""))
		writeConfigFile(t, m.home, "machine.key", content)
		out, err := m.run("agents", "--json")
		if err == nil || !strings.Contains(out, "machine.key") || strings.Contains(out, "hunter2") || strings.Contains(out, strings.TrimSpace(content)) && strings.TrimSpace(content) != "" {
			t.Errorf("%s: %v %q", name, err, safe(string(out)))
		}
	}
}

func TestAKeyFileWithAnExtraLineAfterTheKeyStillWorksBecauseTheReaderTakesTheFirstLine(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	raw := readFileOrFail(t, m.keyPath())
	if err := os.WriteFile(m.keyPath(), []byte(strings.TrimRight(raw, "\n")+"\nan extra line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := m.run("agents", "--json"); err != nil {
		t.Errorf("a key file with a second line: %v %s", err, safe(string(out)))
	}
}

var (
	lowerHexTwelve = regexp.MustCompile(`^[0-9a-f]{12}$`)
	isoDate        = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

func TestMachinesListsEachMachineOnOneLineWithTheNameAFingerprintOfTheRawKeyAndTheCreatedDateAsThreeTabSeparatedFieldsAndNoKey(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	first, second := h.machine("linux-box"), h.machine("mac-mini")
	out := listedMachines(t, h)
	created := map[string]string{}
	for _, entry := range storedEntries(t, readFileOrFail(t, h.dataFile("machines.json"))) {
		name, _ := entry["name"].(string)
		stamp, _ := entry["created"].(string)
		when, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			t.Fatalf("the created field of %s: %v", name, err)
		}
		created[name] = when.UTC().Format("2006-01-02")
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("machines printed %d lines, want 2:\n%s", len(lines), safe(out))
	}
	fields := map[string][]string{}
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			t.Fatalf("the line %q has %d tab separated fields, want 3", line, len(parts))
		}
		fields[parts[0]] = parts
	}
	for _, m := range []*machineRig{first, second} {
		parts, listed := fields[m.name]
		if !listed {
			t.Fatalf("%s is not the first field of any line of:\n%s", m.name, safe(out))
		}
		raw := sha256.Sum256(m.publicKey())
		if want := hex.EncodeToString(raw[:])[:12]; parts[1] != want || !lowerHexTwelve.MatchString(parts[1]) {
			t.Errorf("the fingerprint field for %s is %q, want %q, the first 12 lower case hex characters of the SHA-256 of its 32 key bytes", m.name, parts[1], want)
		}
		if !isoDate.MatchString(parts[2]) || parts[2] != created[m.name] {
			t.Errorf("the date field for %s is %q, want exactly %q shaped YYYY-MM-DD", m.name, parts[2], created[m.name])
		}
		for _, key := range []string{base64.StdEncoding.EncodeToString(m.publicKey()), hex.EncodeToString(m.publicKey()), strings.TrimSpace(readFileOrFail(t, m.keyPath()))} {
			if strings.Contains(out, key) {
				t.Errorf("machines printed a key for %s", m.name)
			}
		}
	}
}

func TestMachinesWithNoMachinesSucceedsAndListsNoneAndNotAPendingInvite(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.invite("pending-box")
	if out := listedMachines(t, h); twelveHex.MatchString(out) {
		t.Errorf("machines listed a machine that does not exist:\n%s", safe(string(out)))
	}
}

var streamClient = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 10 * time.Second}}

func openStreamWith(t *testing.T, h *hostRig, token string) chan struct{} {
	t.Helper()
	req, _ := http.NewRequest("GET", h.url+"/v1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := streamClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream before revoke: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	ended := make(chan struct{})
	go func() { io.Copy(io.Discard, resp.Body); close(ended) }()
	select {
	case <-ended:
		t.Fatal("the stream closed before the revoke")
	case <-time.After(500 * time.Millisecond):
	}
	return ended
}

func TestRevokeRemovesTheMachineAndEndsItsKeyItsAgentTokensItsObserverTokensAndItsStreamsAtOnceAndForGoodAfterARestart(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	agentToken, observer := m.agentToken("alpha"), h.observerToken(m)
	if !h.tokenWorks(agentToken) || !h.tokenWorks(observer) {
		t.Fatal("a token did not work before the revoke")
	}
	ended := openStreamWith(t, h, agentToken)
	for _, partial := range []string{"inux-bo", "linux", "linux-bo", "LINUX-BOX"} {
		if out, err := h.run("server", "revoke", partial); err == nil {
			t.Errorf("revoke of %q, which is not a machine's name, succeeded: %s", partial, safe(string(out)))
		}
	}
	if !h.tokenWorks(agentToken) {
		t.Fatal("a refused revoke ended a token")
	}
	if out, err := h.run("server", "revoke", "linux-box"); err != nil {
		t.Fatalf("revoke: %v %s", err, safe(string(out)))
	}
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Error("the machine's open stream stayed open after revoke")
	}
	if h.tokenWorks(agentToken) {
		t.Error("the agent token still works after revoke")
	}
	if h.tokenWorks(observer) {
		t.Error("the observer token still works after revoke")
	}
	if strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the machine is still listed")
	}
	if status := h.signedStatus(m, time.Now().Unix(), "POST", "/v1/register", registerBody("bravo")); !refused(status) {
		t.Errorf("a signed registration after revoke answered %d", status)
	}
	if out, err := m.run("register", "--id", "charlie", "--harness", "claude-code", "--host", "mac"); err == nil {
		t.Errorf("the machine registered after revoke: %s", safe(string(out)))
	}
	h.restartAndSettle()
	if strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the machine came back after a restart")
	}
	if status := h.signedStatus(m, time.Now().Unix(), "POST", "/v1/register", registerBody("delta")); !refused(status) {
		t.Errorf("a signed registration after revoke and a restart answered %d", status)
	}
}

func TestRevokeLeavesOtherMachinesTheirTokensAndStreamsAndAgentsRegisteredWithTheSharedSecretWorking(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("gone-box")
	kept := h.machine("kept-box")
	kept.registerAgent("alpha")
	keptToken, keptObserver := kept.agentToken("alpha"), h.observerToken(kept)
	keptStream := openStreamWith(t, h, keptToken)
	if out, err := h.run("register", "--id", "local", "--harness", "claude-code", "--host", "mac"); err != nil {
		t.Fatalf("local register: %v %s", err, safe(string(out)))
	}
	localToken := h.hostToken("local")
	if out, err := h.run("server", "revoke", "gone-box"); err != nil {
		t.Fatalf("revoke: %v %s", err, safe(string(out)))
	}
	if !h.tokenWorks(localToken) {
		t.Error("a revoke ended the token of an agent registered with the shared secret")
	}
	if !h.tokenWorks(keptToken) || !h.tokenWorks(keptObserver) {
		t.Error("another machine's token stopped working after a revoke")
	}
	select {
	case <-keptStream:
		t.Error("another machine's stream was closed by a revoke")
	case <-time.After(500 * time.Millisecond):
	}
	listed := listedMachines(t, h)
	if strings.Contains(listed, "gone-box") || !strings.Contains(listed, "kept-box") {
		t.Errorf("machines after the revoke:\n%s", listed)
	}
	if out, err := h.run("agents", "--json"); err != nil || !strings.Contains(out, "local") {
		t.Errorf("an agent registered with the shared secret: %v %s", err, safe(string(out)))
	}
}

func TestRevokeWithAnUnknownNameFailsWithExitOneAndChangesNothing(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	if out, err := h.run("server", "revoke", "no-such-box"); exitCodeOf(err) != 1 {
		t.Fatalf("revoke of an unknown name: %v %q", err, safe(string(out)))
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("an unknown revoke removed a machine")
	}
	if out, err := h.run("server", "revoke"); err == nil {
		t.Errorf("revoke with no name and no --all succeeded: %s", safe(string(out)))
	}
}

func TestRevokeAllRemovesEveryMachineAndEndsEveryKeyEveryTokenAndEveryStream(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	machines := []*machineRig{h.machine("one-box"), h.machine("two-box")}
	if out, err := h.run("register", "--id", "localagent", "--harness", "claude-code", "--host", "mac"); err != nil {
		t.Fatalf("local register: %v %s", err, safe(string(out)))
	}
	var tokens []string
	var ended []chan struct{}
	for i, m := range machines {
		id := fmt.Sprintf("agent%d", i)
		m.registerAgent(id)
		agentToken := m.agentToken(id)
		tokens = append(tokens, agentToken, h.observerToken(m))
		ended = append(ended, openStreamWith(t, h, agentToken))
	}
	if out, err := h.run("server", "revoke", "--all"); err != nil {
		t.Fatalf("revoke --all: %v %s", err, safe(string(out)))
	}
	if twelveHex.MatchString(listedMachines(t, h)) {
		t.Error("machines remain after revoke --all")
	}
	for _, token := range tokens {
		if h.tokenWorks(token) {
			t.Error("a token of a revoked machine still works")
		}
	}
	for _, done := range ended {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("a stream of a revoked machine stayed open")
		}
	}
	for _, m := range machines {
		if out, err := m.run("agents", "--json"); err == nil {
			t.Errorf("%s still works after revoke --all: %s", m.name, safe(string(out)))
		}
	}
	if out, err := h.run("agents", "--json"); err != nil || !strings.Contains(out, "localagent") {
		t.Errorf("an agent registered with the shared secret: %v %s", err, safe(string(out)))
	}
}

func TestARevokedNameCanBeInvitedAndJoinedAgain(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	if out, err := h.run("server", "revoke", "linux-box"); err != nil {
		t.Fatalf("revoke: %v %s", err, safe(string(out)))
	}
	again := h.machine("linux-box")
	again.registerAgent("alpha")
}

func TestRotateSecretLeavesMachinesWorkingWithoutAJoinAndTheirMCPProcessesRegisterAgain(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	cmd := exec.Command(mcpOf(h.bin))
	cmd.Dir, cmd.Env = m.home, m.env
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	p.tool("landline_register", `{"project":"remote-proj"}`)
	if got := p.tool("landline_agents", `{}`); !strings.Contains(got, "project: remote-proj") {
		t.Fatalf("before the rotation the list does not show the session: %s", got)
	}
	before := readFileOrFail(t, m.keyPath())
	out, err := h.run("server", "rotate-secret")
	if err != nil || !strings.Contains(out, "rotated the switchboard secret") {
		t.Fatalf("rotate-secret: %v %s", err, h.clean(out))
	}
	if readFileOrFail(t, m.keyPath()) != before {
		t.Error("rotation changed the machine's key file")
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("rotation removed the machine")
	}
	if out, err := m.run("agents", "--json"); err != nil {
		t.Errorf("the machine needed a join after rotation: %v %s", err, safe(string(out)))
	}
	if got := p.tool("landline_agents", `{}`); !strings.Contains(got, "project: remote-proj") {
		t.Errorf("the machine's MCP process did not register again after the rotation: %s", got)
	}
}

func TestASignedRegisterObserverLogAndBuildRequestIsAccepted(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	for _, each := range []struct {
		method, path string
		body         []byte
	}{
		{"POST", "/v1/register", registerBody("alpha")},
		{"POST", "/v1/observers", []byte(`{"kind":"tui"}`)},
		{"GET", "/v1/log", nil},
		{"GET", "/v1/build", nil},
	} {
		if status, _ := h.signed(m, unix, each.method, each.path, each.body); status != http.StatusOK {
			t.Errorf("signed %s %s answered %d", each.method, each.path, status)
		}
	}
}

func TestASignedRequestSentTwiceIsRefusedTheSecondTimeOnEveryRouteAMachineSignatureOpens(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	for _, each := range []struct {
		method, path string
		body         []byte
	}{
		{"GET", "/v1/build", nil}, {"GET", "/v1/log", nil},
		{"POST", "/v1/observers", []byte(`{"kind":"tui"}`)}, {"POST", "/v1/register", registerBody("alpha")},
	} {
		headers := map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, each.method, each.path, each.body)}
		if status := h.statusOnly(each.method, each.path, each.body, headers); status != http.StatusOK {
			t.Fatalf("%s %s the first time: %d", each.method, each.path, status)
		}
		if status := h.statusOnly(each.method, each.path, each.body, headers); !refused(status) {
			t.Errorf("%s %s the second time, a replay: %d", each.method, each.path, status)
		}
	}
}

func TestTwoRequestsThatDifferOnlyInTheNonceAreBothAcceptedAndAnExactCopyOfOneIsNot(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	body := []byte(`{"kind":"tui"}`)
	first := map[string]string{"Authorization": machineAuthWith(m.key(), m.name, unix, "AAAAAAAAAAAAAAAAAAAAAA==", "POST", "/v1/observers", body)}
	second := map[string]string{"Authorization": machineAuthWith(m.key(), m.name, unix, "AQEBAQEBAQEBAQEBAQEBAQ==", "POST", "/v1/observers", body)}
	for label, headers := range map[string]map[string]string{"first": first, "second": second} {
		if status := h.statusOnly("POST", "/v1/observers", body, headers); status != http.StatusOK {
			t.Errorf("the %s request answered %d", label, status)
		}
	}
	if status := h.statusOnly("POST", "/v1/observers", body, first); !refused(status) {
		t.Errorf("an exact copy of the first request answered %d", status)
	}
}

func TestASignatureThatLeavesTheNonceOutOfItsTextIsRefused(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	nonce := freshNonce()
	text := signedText(m.name, unix, nonce, "GET", "/v1/build", nil)
	without := append(append([]string{}, text[:3]...), text[4:]...)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(m.key(), []byte(strings.Join(without, "\n"))))
	headers := map[string]string{"Authorization": machineHeader(m.name, unix, nonce, signature)}
	if status := h.statusOnly("GET", "/v1/build", nil, headers); !refused(status) {
		t.Errorf("a signature that does not cover the nonce answered %d", status)
	}
}

func TestASignedRequestIsRefusedAgainAfterARestartEvenWithinItsWindow(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	headers := map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, "GET", "/v1/build", nil)}
	if status := h.statusOnly("GET", "/v1/build", nil, headers); status != http.StatusOK {
		t.Fatalf("the first time: %d", status)
	}
	time.Sleep(1100 * time.Millisecond)
	h.restartAndSettle()
	if status := h.statusOnly("GET", "/v1/build", nil, headers); !refused(status) {
		t.Errorf("a signature from before the restart answered %d after it", status)
	}
	if status := h.signedStatus(m, time.Now().Unix(), "GET", "/v1/build", nil); status != http.StatusOK {
		t.Errorf("a fresh signature after the restart answered %d, so the machine did not survive", status)
	}
}

func TestARequestTimedInTheSecondOfAStartIsAcceptedRightAfterEachOfThreeRestarts(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	for i := 0; i < 3; i++ {
		h.restartAndSettle()
		if status := h.signedStatus(m, time.Now().Unix(), "GET", "/v1/build", nil); status != http.StatusOK {
			t.Errorf("restart %d: a request signed at once answered %d", i+1, status)
		}
	}
}

func TestTheSignedPathIncludesTheQueryString(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	path := "/v1/log?after=0"
	good := map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, "GET", path, nil)}
	if status := h.statusOnly("GET", path, nil, good); status != http.StatusOK {
		t.Errorf("a signature over the path and its query: %d", status)
	}
	bare := map[string]string{"Authorization": machineAuth(m.key(), m.name, unix+1, "GET", "/v1/log", nil)}
	if status := h.statusOnly("GET", path, nil, bare); !refused(status) {
		t.Errorf("a signature over the path alone, sent with a query: %d", status)
	}
}

func TestASignatureThatCoversAnotherBodyPathMethodNameOrKeyIsRefused(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	other := h.machine("other-box")
	unix := time.Now().Unix()
	body := registerBody("alpha")
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	cases := map[string]struct {
		method, path, name string
		sent               []byte
		auth               string
	}{
		"another body":     {"POST", "/v1/register", m.name, registerBody("mallory"), machineAuth(m.key(), m.name, unix, "POST", "/v1/register", body)},
		"another path":     {"POST", "/v1/observers", m.name, body, machineAuth(m.key(), m.name, unix, "POST", "/v1/register", body)},
		"another method":   {"GET", "/v1/log", m.name, nil, machineAuth(m.key(), m.name, unix, "POST", "/v1/log", nil)},
		"another name":     {"POST", "/v1/register", other.name, body, strings.Replace(machineAuth(m.key(), m.name, unix, "POST", "/v1/register", body), "name="+m.name, "name="+other.name, 1)},
		"a stranger's key": {"POST", "/v1/register", m.name, body, machineAuth(stranger, m.name, unix, "POST", "/v1/register", body)},
		"an unknown name":  {"POST", "/v1/register", "no-such-box", body, machineAuth(m.key(), "no-such-box", unix, "POST", "/v1/register", body)},
	}
	for name, c := range cases {
		if status := h.statusOnly(c.method, c.path, c.sent, map[string]string{"Authorization": c.auth}); !refused(status) {
			t.Errorf("%s answered %d", name, status)
		}
	}
}

func TestATimeMoreThanTheDefaultFiveSecondsAheadIsRefusedWithAMessageThatNamesTheClockAndTheGapAndFourIsAccepted(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	status, body := h.signed(m, time.Now().Unix()+9, "POST", "/v1/register", registerBody("alpha"))
	if status != http.StatusUnauthorized || !strings.Contains(strings.ToLower(body), "clock") || !regexp.MustCompile(`(^|\D)([7-9])(\D|$)`).MatchString(body) {
		t.Errorf("a time 9 seconds ahead under the default window: %d %s, want 401 with a message that names the clock and a difference of about 9 seconds", status, body)
	}
	if status := h.signedStatus(m, time.Now().Unix()+4, "POST", "/v1/register", registerBody("bravo")); status != http.StatusOK {
		t.Errorf("a time 4 seconds ahead under the default window: %d", status)
	}
}

func TestATimeBeyondTheSignatureWindowBehindIsRefusedWithTheClockMessageAndInsideTheWindowIsAccepted(t *testing.T) {
	t.Parallel()
	h := startHostWith(t, "--signature-window", "5s")
	m := h.machine("linux-box")
	time.Sleep(10 * time.Second)
	status, body := h.signed(m, time.Now().Unix()-8, "POST", "/v1/register", registerBody("alpha"))
	if status != http.StatusUnauthorized || !strings.Contains(strings.ToLower(body), "clock") || !regexp.MustCompile(`(^|\D)([6-9]|10)(\D|$)`).MatchString(body) {
		t.Errorf("a time 8 seconds behind with a window of 5 seconds: %d, want 401 with the clock message", status)
	}
	if status := h.signedStatus(m, time.Now().Unix()-1, "POST", "/v1/register", registerBody("bravo")); status != http.StatusOK {
		t.Errorf("a time 1 second behind with a window of 5 seconds: %d", status)
	}
}

func TestRestartPassesTheSignatureWindowOn(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	h.restartAndSettle("--signature-window", "3s")
	if status := h.signedStatus(m, time.Now().Unix(), "GET", "/v1/build", nil); status != http.StatusOK {
		t.Errorf("after restart --signature-window 3s, a request timed now answered %d", status)
	}
	if status := h.signedStatus(m, time.Now().Unix()+10, "GET", "/v1/build", nil); !refused(status) {
		t.Errorf("after restart --signature-window 3s, a time 10 seconds ahead answered %d", status)
	}
}

func TestASignatureIsStillRememberedPastOneWindowWhenItsTimeIsAWindowAhead(t *testing.T) {
	t.Parallel()
	h := startHostWith(t, "--signature-window", "4s")
	m := h.machine("linux-box")
	unix := time.Now().Unix() + 3
	headers := map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, "GET", "/v1/build", nil)}
	if status := h.statusOnly("GET", "/v1/build", nil, headers); status != http.StatusOK {
		t.Fatalf("the first time: %d", status)
	}
	time.Sleep(6 * time.Second)
	if status := h.statusOnly("GET", "/v1/build", nil, headers); !refused(status) {
		t.Errorf("a replay 6 seconds later, inside the window and past one window of memory: %d", status)
	}
}

func TestASignatureTimedBeforeARestartIsRefusedAfterItAndOneTimedPastTheStartPeriodIsAccepted(t *testing.T) {
	t.Parallel()
	h := startHostWith(t, "--signature-window", "5s")
	m := h.machine("linux-box")
	early := time.Now().Add(-4 * time.Second).Unix()
	h.restartWithoutWaiting()
	if status := h.signedStatus(m, early, "POST", "/v1/register", registerBody("alpha")); !refused(status) {
		t.Errorf("a signature timed 4 seconds before the switchboard restarted answered %d", status)
	}
	waitPastTheStartPeriod()
	if status := h.signedStatus(m, time.Now().Unix(), "POST", "/v1/register", registerBody("alpha")); status != http.StatusOK {
		t.Errorf("a signature timed now answered %d", status)
	}
}

func (h *hostRig) restartWithoutWaiting(flags ...string) {
	h.t.Helper()
	if out, err := h.run(append([]string{"server", "restart"}, flags...)...); err != nil {
		h.t.Fatalf("restart: %v %s", err, h.clean(out))
	}
}

func TestEverySignedRequestIsRefusedWithReasonRefusedInTheFirstFiveSecondsAfterARestartAndAcceptedAfter(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	before := time.Now()
	h.restartWithoutWaiting()
	if took := time.Since(before); took > 2*time.Second {
		t.Skipf("restart took %s, too long to sign two seconds after its start second and inside its start period", took)
	}
	sleeper.Until(time.Now().Truncate(time.Second).Add(2 * time.Second))
	status, body := h.signed(m, time.Now().Unix(), "POST", "/v1/observers", []byte(`{"kind":"tui"}`))
	var answer struct{ Reason string }
	json.Unmarshal([]byte(body), &answer)
	if status != http.StatusUnauthorized || answer.Reason != "refused" {
		t.Errorf("a request timed two seconds after the start second of a restart: %d %s, want 401 with reason refused", status, body)
	}
	waitPastTheStartPeriod()
	if status := h.signedStatus(m, time.Now().Unix(), "POST", "/v1/observers", []byte(`{"kind":"tui"}`)); status != http.StatusOK {
		t.Errorf("a request timed now, past the start period: %d", status)
	}
}

func TestACopyOfARequestFromAMachineWhoseClockRunsAheadIsRefusedAfterARestart(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	headers := map[string]string{"Authorization": machineAuth(m.key(), m.name, time.Now().Unix()+4, "GET", "/v1/build", nil)}
	if status := h.statusOnly("GET", "/v1/build", nil, headers); status != http.StatusOK {
		t.Fatalf("a time 4 seconds ahead inside the default window of 5 seconds: %d", status)
	}
	h.restartWithoutWaiting()
	if status := h.statusOnly("GET", "/v1/build", nil, headers); !refused(status) {
		t.Errorf("a copy of a request accepted before the restart, from a clock 4 seconds ahead, was accepted after it: %d", status)
	}
}

func TestAMachineSignatureIsRefusedOnEveryRouteOutsideRegisterObserversLogAndBuild(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	unix := time.Now().Unix()
	for _, each := range []struct {
		method, path string
		body         []byte
	}{
		{"POST", "/v1/send", []byte(`{"to":"nobody","body":"x"}`)},
		{"GET", "/v1/stream", nil},
		{"POST", "/v1/status", []byte(`{"task":"x"}`)},
		{"POST", "/v1/attach", []byte(`{"project":"x"}`)},
		{"GET", "/v1/agents", nil},
		{"POST", "/v1/deliveries/x/report", []byte(`{"ok":true}`)},
	} {
		if status := h.signedStatus(m, unix, each.method, each.path, each.body); status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("a machine signature on %s %s answered %d, want 401 or 403", each.method, each.path, status)
		}
	}
}

var machineClient = &http.Client{Timeout: 10 * time.Second}

func (h *hostRig) restartAndSettle(flags ...string) {
	h.t.Helper()
	if out, err := h.run(append([]string{"server", "restart"}, flags...)...); err != nil {
		h.t.Fatalf("restart: %v %s", err, h.clean(out))
	}
	waitPastTheStartPeriod()
}

func waitPastTheStartPeriod() {
	sleeper.Until(time.Now().Truncate(time.Second).Add(6 * time.Second))
}

var hyphenatedCode = regexp.MustCompile(`[2-9A-HJKMNP-Za-hjkmnp-z]{4}-[2-9A-HJKMNP-Za-hjkmnp-z]{4}`)

func (h *hostRig) clean(text string) string {
	if h.secret != "" {
		text = strings.ReplaceAll(text, h.secret, "<shared secret>")
	}
	for _, code := range h.issued {
		text = strings.ReplaceAll(text, code, "<code>")
		text = strings.ReplaceAll(text, strings.ReplaceAll(code, "-", ""), "<code>")
	}
	return hyphenatedCode.ReplaceAllString(text, "<code>")
}

func (h *hostRig) rawRequest(method, path string, body []byte, headers map[string]string) (int, string) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, h.url+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := machineClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String()
}

func (h *hostRig) statusQuiet(method, path string, body []byte, headers map[string]string) int {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, h.url+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := machineClient.Do(req)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (h *hostRig) statusOnly(method, path string, body []byte, headers map[string]string) int {
	h.t.Helper()
	status := h.statusQuiet(method, path, body, headers)
	if status == -1 {
		h.t.Fatalf("%s %s: no answer", method, path)
	}
	return status
}

func (h *hostRig) signedStatus(m *machineRig, unix int64, method, path string, body []byte) int {
	h.t.Helper()
	return h.statusOnly(method, path, body, map[string]string{"Authorization": machineAuth(m.key(), m.name, unix, method, path, body)})
}

func (h *hostRig) adminStatus(method, path string, body []byte) (int, string) {
	h.t.Helper()
	return h.rawRequest(method, path, body, map[string]string{"Authorization": "Bearer " + h.secret})
}

func (h *hostRig) observerToken(m *machineRig) string {
	h.t.Helper()
	status, body := h.signed(m, time.Now().Unix(), "POST", "/v1/observers", []byte(`{"kind":"tui"}`))
	var answer struct{ Token string }
	if status != http.StatusOK || json.Unmarshal([]byte(body), &answer) != nil || answer.Token == "" {
		h.t.Fatalf("a signed observer registration: %d", status)
	}
	return answer.Token
}

func (h *hostRig) tokenWorks(token string) bool {
	h.t.Helper()
	return h.statusOnly("GET", "/v1/agents", nil, map[string]string{"Authorization": "Bearer " + token}) == http.StatusOK
}

type recorded struct {
	method, uri string
	header      http.Header
	body        []byte
}

type recordingProxy struct {
	host, port string
	mu         sync.Mutex
	seen       []recorded
	replies    []string
	holding    bool
}

func (p *recordingProxy) hold(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.holding = on
}

func (p *recordingProxy) held() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.holding
}

func (p *recordingProxy) responses() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.replies...)
}

func startProxy(t *testing.T, target string) *recordingProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &recordingProxy{}
	p.host, p.port, _ = net.SplitHostPort(listener.Addr().String())
	upstream := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: target})
	upstream.FlushInterval = -1
	upstream.ModifyResponse = func(resp *http.Response) error {
		p.mu.Lock()
		p.replies = append(p.replies, fmt.Sprint(resp.Header))
		p.mu.Unlock()
		if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		p.mu.Lock()
		p.replies = append(p.replies, string(body))
		p.mu.Unlock()
		return err
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.seen = append(p.seen, recorded{method: r.Method, uri: r.URL.RequestURI(), header: r.Header.Clone(), body: body})
		p.mu.Unlock()
		if p.held() {
			w.Write([]byte("{}"))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		upstream.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	return p
}

func (p *recordingProxy) requests() []recorded {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]recorded{}, p.seen...)
}

func (p *recordingProxy) forget() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = nil
}

func (r recorded) everything() string {
	var out strings.Builder
	out.WriteString(r.method + " " + r.uri + "\n")
	for name, values := range r.header {
		out.WriteString(name + ": " + strings.Join(values, ",") + "\n")
	}
	out.Write(r.body)
	return out.String()
}

func TestNeitherAMachinesSeedNorTheSharedSecretCrossesTheNetworkWhenItJoinsRegistersAndReads(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	proxy := startProxy(t, h.addr)
	m := h.newMachineHome()
	code := h.invite("linux-box")
	if out, err := m.run("server", "join", net.JoinHostPort(proxy.host, proxy.port), "--code", code); err != nil {
		t.Fatalf("join through the proxy: %v %s", err, h.clean(out))
	}
	m.name = "linux-box"
	m.registerAgent("alpha")
	m.run("agents", "--json")
	m.run("agents", "--json")
	m.run("log")
	cmd := exec.Command(mcpOf(h.bin))
	cmd.Dir, cmd.Env = m.home, m.env
	p := newMCP(t, cmd)
	p.handshake("claude-code", "2.1.296")
	p.tool("landline_register", `{"project":"remote-proj"}`)
	seed := strings.TrimSpace(readFileOrFail(t, m.keyPath()))
	raw, _ := base64.StdEncoding.DecodeString(seed)
	signed := false
	nonces := map[string]bool{}
	for _, each := range proxy.requests() {
		text := each.everything()
		where := each.method + " " + strings.SplitN(each.uri, "?", 2)[0]
		if strings.Contains(text, seed) || strings.Contains(text, hex.EncodeToString(raw)) {
			t.Errorf("%s carried the machine's private seed", where)
		}
		if strings.Contains(text, h.secret) {
			t.Errorf("%s carried the shared secret", where)
		}
		if header := each.header.Get("Authorization"); strings.HasPrefix(header, "Landline-Machine ") {
			signed = true
			nonce := nonceOf(header)
			if raw, err := base64.StdEncoding.DecodeString(nonce); err != nil || len(raw) != 16 {
				t.Errorf("%s carried a nonce that is not 16 bytes of base64", where)
			}
			if nonces[nonce] {
				t.Errorf("%s reused a nonce", where)
			}
			nonces[nonce] = true
		}
	}
	if !signed {
		t.Error("no request carried a machine signature")
	}
	for _, reply := range proxy.responses() {
		if strings.Contains(reply, seed) || strings.Contains(reply, hex.EncodeToString(raw)) {
			t.Error("a reply to the machine carried a private seed")
		}
		if strings.Contains(reply, h.secret) {
			t.Error("a reply to the machine carried the shared secret")
		}
	}
}

func TestAdminCommandsSendTheSharedSecretAndTheSwitchboardRefusesTheSameRequestsSignedByAMachineKey(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	machine := h.machine("linux-box")
	h.machine("other-box")
	proxy := startProxy(t, h.addr)
	admin := h.newMachineHome()
	writeConfigFile(t, admin.home, "config.toml", tomlFor(proxy.host, proxy.port, h.secret)+"machine = \"linux-box\"\n")
	if err := os.WriteFile(admin.keyPath(), []byte(readFileOrFail(t, machine.keyPath())), 0o600); err != nil {
		t.Fatal(err)
	}
	proxy.hold(true)
	for _, args := range [][]string{{"server", "invite", "--name", "probe-box"}, {"server", "machines"}, {"server", "revoke", "other-box"}, {"server", "revoke", "--all"}} {
		proxy.forget()
		admin.run(args...)
		requests := proxy.requests()
		if len(requests) == 0 {
			t.Fatalf("%v sent no request", args)
		}
		for _, each := range requests {
			if each.header.Get("Authorization") != "Bearer "+h.secret {
				t.Errorf("%v sent %s %s with a %q credential, want the shared secret", args, each.method, strings.SplitN(each.uri, "?", 2)[0], schemeOf(each.header.Get("Authorization")))
			}
			unix := time.Now().Unix()
			headers := map[string]string{"Authorization": machineAuth(machine.key(), machine.name, unix, each.method, each.uri, each.body), "Content-Type": "application/json"}
			if status := h.statusOnly(each.method, each.uri, each.body, headers); status != http.StatusUnauthorized && status != http.StatusForbidden {
				t.Errorf("%v: the same request %s %s signed by a machine key answered %d, want 401 or 403", args, each.method, strings.SplitN(each.uri, "?", 2)[0], status)
			}
		}
	}
	listed := listedMachines(t, h)
	if !strings.Contains(listed, "linux-box") || !strings.Contains(listed, "other-box") || strings.Contains(listed, "probe-box") {
		t.Errorf("machines after the replays:\n%s", listed)
	}
}

func TestAnInviteExpiresAfterTheInviteLifetimeAndAnExpiredCodeLooksLikeAWrongOneAndItsNameIsFreeAgain(t *testing.T) {
	t.Parallel()
	h := startHostWith(t, "--invite-lifetime", "5s")
	quick := h.invite("quick-box")
	if out, err := h.newMachineHome().joinWith(quick); err != nil {
		t.Fatalf("a code used at once: %v %s", err, h.clean(out))
	}
	expired := h.invite("slow-box")
	time.Sleep(6 * time.Second)
	wrongCode := randomCode(t)
	out, err := h.newMachineHome().joinWith(expired)
	wrongOut, wrongErr := h.newMachineHome().joinWith(wrongCode)
	if err == nil || wrongErr == nil || !strings.Contains(out, invalidCode) || !strings.Contains(wrongOut, invalidCode) {
		t.Fatalf("an expired code: %v %q, a wrong code: %v %q", err, h.clean(out), wrongErr, h.clean(wrongOut))
	}
	if strings.ReplaceAll(strings.TrimSpace(out), expired, "") != strings.ReplaceAll(strings.TrimSpace(wrongOut), wrongCode, "") {
		t.Errorf("an expired code and a wrong code are told apart:\n%q\n%q", h.clean(out), h.clean(wrongOut))
	}
	h.invite("slow-box")
}

func TestRestartPassesTheTimingFlagsOn(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.restartAndSettle("--invite-lifetime", "2s")
	expired := h.invite("first-box")
	time.Sleep(3 * time.Second)
	if out, err := h.newMachineHome().joinWith(expired); err == nil || !strings.Contains(out, invalidCode) {
		t.Errorf("after restart --invite-lifetime 2s, a 3 second old code: %v %q", err, h.clean(out))
	}
}

func TestTheAdministrationRoutesTakeTheSharedSecretAndRefuseAMachineSignatureAWrongBearerOrNothing(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	routes := []struct {
		method, path string
		body         []byte
	}{
		{"POST", "/v1/machines", []byte(`{"name":"route-box"}`)},
		{"GET", "/v1/machines", nil},
		{"DELETE", "/v1/machines/linux-box", nil},
		{"DELETE", "/v1/machines", nil},
	}
	unix := time.Now().Unix()
	for _, route := range routes {
		for label, headers := range map[string]map[string]string{
			"no credential":       {},
			"a wrong bearer":      {"Authorization": "Bearer " + strings.Repeat("x", 32)},
			"a machine signature": {"Authorization": machineAuth(m.key(), m.name, unix, route.method, route.path, route.body)},
		} {
			if status := h.statusOnly(route.method, route.path, route.body, headers); status != http.StatusUnauthorized && status != http.StatusForbidden {
				t.Errorf("%s %s with %s answered %d, want 401 or 403", route.method, route.path, label, status)
			}
		}
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Fatal("a refused request revoked the machine")
	}
	status, body := h.adminStatus("POST", "/v1/machines", []byte(`{"name":"route-box"}`))
	var invite struct{ Name, Code string }
	if status < 200 || status > 299 || json.Unmarshal([]byte(body), &invite) != nil || invite.Name != "route-box" || !regexp.MustCompile(`^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$`).MatchString(invite.Code) {
		t.Fatalf("POST /v1/machines with the secret: %d", status)
	}
	if status, _ := h.adminStatus("GET", "/v1/machines", nil); status != http.StatusOK {
		t.Errorf("GET /v1/machines with the secret: %d", status)
	}
	if status, _ := h.adminStatus("DELETE", "/v1/machines/no-such-box", nil); status != http.StatusNotFound {
		t.Errorf("DELETE of an unknown name: %d, want 404", status)
	}
	if status, _ := h.adminStatus("DELETE", "/v1/machines/route-box", nil); status < 200 || status > 299 {
		t.Errorf("DELETE of a pending invite: %d", status)
	}
	if status, _ := h.adminStatus("DELETE", "/v1/machines/linux-box", nil); status < 200 || status > 299 {
		t.Errorf("DELETE of a machine: %d", status)
	}
	h.machine("again-box")
	if status, _ := h.adminStatus("DELETE", "/v1/machines", nil); status < 200 || status > 299 {
		t.Errorf("DELETE of every machine: %d", status)
	}
	if twelveHex.MatchString(listedMachines(t, h)) {
		t.Error("machines remain after DELETE /v1/machines")
	}
}

var anyCode = regexp.MustCompile(`[2-9A-HJKMNP-Za-hjkmnp-z]{4}-[2-9A-HJKMNP-Za-hjkmnp-z]{4}`)

func safe(text string) string { return anyCode.ReplaceAllString(text, "<code>") }

func schemeOf(header string) string {
	switch {
	case strings.HasPrefix(header, "Bearer "):
		return "Bearer"
	case strings.HasPrefix(header, "Landline-Machine "):
		return "Landline-Machine"
	case header == "":
		return "no"
	}
	return "an unknown"
}

func (h *hostRig) hostToken(id string) string {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(configDirOf(h.home), "session-"+id+".json"))
	if err != nil {
		h.t.Fatal(err)
	}
	var session struct{ Token string }
	json.Unmarshal(raw, &session)
	return session.Token
}

func TestRevokeAllClearsPendingInvitesAndFreesTheirNamesAndLeavesTheSharedSecretAgentsAlone(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("active-box")
	pending := h.invite("pending-box")
	if out, err := h.run("register", "--id", "local", "--harness", "claude-code", "--host", "mac"); err != nil {
		t.Fatalf("local register: %v %s", err, safe(string(out)))
	}
	token := h.hostToken("local")
	if out, err := h.run("server", "revoke", "--all"); err != nil {
		t.Fatalf("revoke --all: %v %s", err, safe(string(out)))
	}
	if out, err := h.newMachineHome().joinWith(pending); err == nil || !strings.Contains(out, invalidCode) {
		t.Errorf("a pending code survived revoke --all: %v %q", err, h.clean(out))
	}
	h.invite("pending-box")
	if !h.tokenWorks(token) {
		t.Error("revoke --all ended the token of an agent registered with the shared secret")
	}
}

func TestAdminCommandsSendTheEnvironmentSecretWhenAKeyAMachineNameAndAStaleFileSecretArePresent(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	machine := h.machine("linux-box")
	proxy := startProxy(t, h.addr)
	proxy.hold(true)
	admin := h.newMachineHome()
	writeConfigFile(t, admin.home, "config.toml", tomlFor(proxy.host, proxy.port, strings.Repeat("y", 32))+"machine = \"linux-box\"\n")
	if err := os.WriteFile(admin.keyPath(), []byte(readFileOrFail(t, machine.keyPath())), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(append([]string{}, admin.env...), "LANDLINE_SECRET="+h.secret)
	for _, args := range [][]string{{"server", "invite", "--name", "probe-box"}, {"server", "machines"}, {"server", "revoke", "linux-box"}, {"server", "revoke", "--all"}} {
		proxy.forget()
		runIn(t, h.bin, env, admin.home, args...)
		requests := proxy.requests()
		if len(requests) == 0 {
			t.Fatalf("%v sent no request", args)
		}
		for _, each := range requests {
			if each.header.Get("Authorization") != "Bearer "+h.secret {
				t.Errorf("%v sent %s %s with %s credentials, want the shared secret from the environment", args, each.method, strings.SplitN(each.uri, "?", 2)[0], schemeOf(each.header.Get("Authorization")))
			}
		}
	}
}

func refused(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func nonceOf(header string) string {
	found := regexp.MustCompile(`nonce=([^,\s]+)`).FindStringSubmatch(header)
	if found == nil {
		return ""
	}
	return found[1]
}

func TestTheInviteRouteRefusesABadNameWith400AndTakesTheValidOnes(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	for _, name := range []string{"", "Linux", "1box", "-box", "has space", "under_score", strings.Repeat("a", 33)} {
		body, _ := json.Marshal(map[string]string{"name": name})
		if status, _ := h.adminStatus("POST", "/v1/machines", body); status != http.StatusBadRequest {
			t.Errorf("name %q answered %d, want 400", name, status)
		}
	}
	for _, name := range []string{"a", "a-", "a--b", strings.Repeat("a", 32)} {
		body, _ := json.Marshal(map[string]string{"name": name})
		if status, _ := h.adminStatus("POST", "/v1/machines", body); status < 200 || status > 299 {
			t.Errorf("name %q answered %d", name, status)
		}
	}
	for _, body := range []string{``, `not json`, `{}`, `{"name":5}`} {
		if status, _ := h.adminStatus("POST", "/v1/machines", []byte(body)); status != http.StatusBadRequest {
			t.Errorf("body %q answered %d, want 400", body, status)
		}
	}
}

func TestTwoInvitesGetDifferentCodes(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	seen := map[string]bool{}
	for _, name := range []string{"one-box", "two-box", "three-box", "four-box", "five-box"} {
		code := h.invite(name)
		if seen[code] {
			t.Fatal("two invites got the same code")
		}
		seen[code] = true
	}
}

func TestAJoinToAnHTTPSAddressIsRefusedAndChangesNoFile(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("control-box")
	m := h.newMachineHome()
	out, err := m.run("server", "join", "https://"+h.addr, "--code", randomCode(t))
	if err == nil || !onlyHTTP.MatchString(out) {
		t.Errorf("join to an https address: %v %q", err, safe(out))
	}
	if _, statErr := os.Stat(m.keyPath()); statErr == nil {
		t.Error("machine.key was written")
	}
	if _, statErr := os.Stat(configTomlOf(m.home)); statErr == nil {
		t.Error("config.toml was written")
	}
}

func TestTheFailureCountStartsAgainAfterACancel(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.invite("first-box")
	failRedeems(t, h, 5)
	next := h.invite("second-box")
	failRedeems(t, h, 4)
	if out, err := h.newMachineHome().joinWith(next); err != nil {
		t.Errorf("four failures after a cancel cancelled the next invite: %v %s", err, h.clean(out))
	}
}

func TestASignedRequestWhoseNonceIsNotSixteenBytesOfStandardBase64Gets401AndAValidOneGets200(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	send := func(nonce string) int {
		status, _ := h.rawRequest("GET", "/v1/build", nil, map[string]string{"Authorization": machineAuthWith(m.key(), m.name, time.Now().Unix(), nonce, "GET", "/v1/build", nil)})
		return status
	}
	if status := send(freshNonce()); status != 200 {
		t.Fatalf("control with a 16 byte nonce: %d", status)
	}
	short, long := base64.StdEncoding.EncodeToString(make([]byte, 15)), base64.StdEncoding.EncodeToString(make([]byte, 17))
	for name, nonce := range map[string]string{"one character": "-", "one byte": "AA==", "not base64": "!!!not-base64!!!", "15 bytes": short, "17 bytes": long, "4000 characters": strings.Repeat("A", 4000)} {
		if status := send(nonce); status != 401 {
			t.Errorf("%s: status %d, want 401", name, status)
		}
	}
}

func TestAJoinThatCannotSaveAfterTheCodeWasRedeemedKeepsTheEarlierKeyAndConfigAndSaysHowToRevokeTheNewMachine(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("alpha")
	keyBefore, configBefore := readFileOrFail(t, m.keyPath()), readFileOrFail(t, configTomlOf(m.home))
	if err := os.MkdirAll(filepath.Join(configDirOf(m.home), "config.toml.tmp", "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := m.joinWith(h.invite("beta"))
	if exitCodeOf(err) != 1 {
		t.Fatalf("exit %d, want 1: %s", exitCodeOf(err), safe(out))
	}
	if readFileOrFail(t, m.keyPath()) != keyBefore || readFileOrFail(t, configTomlOf(m.home)) != configBefore {
		t.Error("the earlier key or config.toml changed")
	}
	if !strings.Contains(out, "landline server revoke beta") {
		t.Errorf("the failure does not say how to revoke the stranded machine: %s", safe(out))
	}
	os.RemoveAll(filepath.Join(configDirOf(m.home), "config.toml.tmp"))
	if out, err := m.run("agents", "--json"); err != nil {
		t.Errorf("the earlier key stopped working: %v %s", err, safe(out))
	}
}

func TestAJoinAgainstASwitchboardThatAnswersWithAMachineNameBreakingTheNameRuleSavesNothing(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	hostile := "a\"\nhost = \"evil.example\"\nsecret = \"x"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		raw, _ := json.Marshal(map[string]string{"name": hostile})
		w.Write(raw)
	}))
	t.Cleanup(fake.Close)
	m := h.newMachineHome()
	out, err := m.run("server", "join", strings.TrimPrefix(fake.URL, "http://"), "--code", "ABCD-EFGH")
	if exitCodeOf(err) != 1 {
		t.Fatalf("exit %d, want 1: %s", exitCodeOf(err), safe(out))
	}
	for _, path := range []string{m.keyPath(), configTomlOf(m.home)} {
		if _, statErr := os.Stat(path); statErr == nil {
			t.Errorf("%s was written", path)
		}
	}
}

func TestAdministrationCommandsRefuseWhenLandlineURLNamesAnotherMachineAndLeaveTheSwitchboardAlone(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	env := append(append([]string{}, h.env...), "LANDLINE_URL=http://203.0.113.9:8787", "LANDLINE_SECRET="+h.secret)
	for _, args := range [][]string{{"server", "invite", "--name", "other-box"}, {"server", "machines"}, {"server", "revoke", "linux-box"}, {"server", "revoke", "--all"}} {
		out, err := runIn(t, h.bin, env, h.home, args...)
		lower := strings.ToLower(out)
		if exitCodeOf(err) != 1 || !strings.Contains(out, "203.0.113.9") || joinCommandLine.MatchString(strings.TrimSpace(out)) ||
			strings.Contains(lower, "dial") || strings.Contains(lower, "timeout") || strings.Contains(lower, "refused by the switchboard") {
			t.Errorf("%v: exit %d, want 1 with a refusal that names the address: %q", args, exitCodeOf(err), safe(out))
		}
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the machine was revoked")
	}
}

func signedRegisterHeaders(t *testing.T, name string, priv ed25519.PrivateKey, unix int64) map[string]string {
	t.Helper()
	return map[string]string{"Authorization": machineAuth(priv, name, unix, "POST", "/v1/register", registerBody("alpha"))}
}

func TestAnUnknownNameSignedWithATimeFarOutsideTheWindowGetsTheSameStatusAndBodyAsAKnownMachine(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	_, ghost, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int64{5000, -5000} {
		var knownStatus, ghostStatus int
		var knownBody, ghostBody string
		for attempt := 0; attempt < 5; attempt++ {
			unix := time.Now().Unix() + offset
			knownStatus, knownBody = h.rawRequest("POST", "/v1/register", registerBody("alpha"), signedRegisterHeaders(t, m.name, m.key(), unix))
			ghostStatus, ghostBody = h.rawRequest("POST", "/v1/register", registerBody("alpha"), signedRegisterHeaders(t, "ghost-box", ghost, unix))
			if knownBody == ghostBody {
				break
			}
		}
		if knownStatus != http.StatusUnauthorized || !strings.Contains(strings.ToLower(knownBody), "clock") {
			t.Fatalf("a known machine with a time %+d seconds off: %d %q, want 401 with the clock message", offset, knownStatus, knownBody)
		}
		if ghostStatus != knownStatus || ghostBody != knownBody {
			t.Errorf("a name that never joined with a time %+d seconds off: %d %q, want the status and body a known machine gets: %d %q", offset, ghostStatus, ghostBody, knownStatus, knownBody)
		}
	}
}

func TestAnUnknownNameSignedWithAGoodTimeGets401AndTheSameGenericRefusalAsAKnownNameWithAWrongKey(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	_, ghost, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unix := time.Now().Unix()
	ghostStatus, ghostBody := h.rawRequest("POST", "/v1/register", registerBody("alpha"), signedRegisterHeaders(t, "ghost-box", ghost, unix))
	wrongStatus, wrongBody := h.rawRequest("POST", "/v1/register", registerBody("alpha"), signedRegisterHeaders(t, m.name, other, unix))
	if ghostStatus != http.StatusUnauthorized || strings.Contains(strings.ToLower(ghostBody), "clock") || strings.Contains(ghostBody, "ghost-box") {
		t.Errorf("a name that never joined with a good time: %d %q, want 401 with the generic machine refusal and no clock message or name", ghostStatus, ghostBody)
	}
	if wrongStatus != ghostStatus || wrongBody != ghostBody {
		t.Errorf("a known name with a wrong key: %d %q, want the same status and body as the unknown name: %d %q", wrongStatus, wrongBody, ghostStatus, ghostBody)
	}
}

func changedFiles(before, after map[string]string) []string {
	var changed []string
	for path, content := range before {
		if now, kept := after[path]; !kept || now != content {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, had := before[path]; !had {
			changed = append(changed, path)
		}
	}
	return changed
}

var refusalOnly = regexp.MustCompile(`^([a-z ]+: )?` + regexp.QuoteMeta(invalidCode) + `$`)

func TestRevokeAllIsSavedSoNoMachineIsListedOrCanSignAfterARestartOfTheSwitchboard(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	machines := []*machineRig{h.machine("one-box"), h.machine("two-box")}
	for _, m := range machines {
		m.registerAgent("alpha")
	}
	if out, err := h.run("server", "revoke", "--all"); err != nil {
		t.Fatalf("revoke --all: %v %s", err, safe(string(out)))
	}
	if entries := storedEntries(t, readFileOrFail(t, h.dataFile("machines.json"))); len(entries) != 0 {
		t.Errorf("machines.json holds %d machines right after revoke --all", len(entries))
	}
	h.restartAndSettle()
	if twelveHex.MatchString(listedMachines(t, h)) {
		t.Error("a machine came back after a restart that followed revoke --all")
	}
	if entries := storedEntries(t, readFileOrFail(t, h.dataFile("machines.json"))); len(entries) != 0 {
		t.Errorf("machines.json holds %d machines after a restart", len(entries))
	}
	for _, m := range machines {
		if status := h.signedStatus(m, time.Now().Unix(), "POST", "/v1/register", registerBody("bravo")); !refused(status) {
			t.Errorf("%s signed a registration after revoke --all and a restart: %d", m.name, status)
		}
		if out, err := m.run("agents", "--json"); err == nil {
			t.Errorf("%s still works after revoke --all and a restart: %s", m.name, safe(string(out)))
		}
	}
}

func TestAFailedJoinWithACodePrintsTheSwitchboardsRefusalAndNothingMoreAboutWhyAndNeverEchoesTheCode(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.invite("linux-box")
	wrong := randomCode(t)
	for _, typed := range []string{wrong, strings.ToLower(wrong), strings.ReplaceAll(wrong, "-", "")} {
		out, err := h.newMachineHome().joinWith(typed)
		if err == nil {
			t.Fatalf("a join with the wrong code %q succeeded", typed)
		}
		line := strings.TrimSuffix(out, "\n")
		if !refusalOnly.MatchString(line) {
			t.Errorf("a failed join with %q printed %q, want only the switchboard's message %q", typed, line, invalidCode)
		}
		for _, echoed := range []string{wrong, strings.ToLower(wrong), strings.ReplaceAll(wrong, "-", ""), strings.ToLower(strings.ReplaceAll(wrong, "-", ""))} {
			if strings.Contains(out, echoed) {
				t.Errorf("a failed join echoed the code %q", echoed)
			}
		}
		for _, status := range []string{"401", "unauthorized", "status"} {
			if strings.Contains(strings.ToLower(out), status) {
				t.Errorf("a failed join printed %q: %q", status, line)
			}
		}
	}
}

func TestAKeyFileWithExtraLinesStillWorksForLogAndRegisterAndAMalformedFirstLineIsAnErrorThatNamesTheFileForEveryCommand(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	seed := strings.TrimSpace(readFileOrFail(t, m.keyPath()))
	commands := map[string][]string{
		"log":      {"log"},
		"register": {"register", "--id", "alpha", "--harness", "claude-code", "--host", "mac"},
	}
	for label, content := range map[string]string{
		"two extra lines":          seed + "\nan extra line\nanother-extra-hunter2\n",
		"a blank line then a line": seed + "\n\nafter the blank\n",
		"no final line feed":       seed + "\nan extra line",
	} {
		if err := os.WriteFile(m.keyPath(), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		for command, args := range commands {
			if out, err := m.run(args...); err != nil {
				t.Errorf("%s with a key file that has %s: %v %s", command, label, err, safe(string(out)))
			}
		}
	}
	for label, content := range map[string]string{
		"garbage first line":      "garbage-first-line-hunter2!!\n" + seed + "\n",
		"blank first line":        "\n" + seed + "\n",
		"short seed first line":   base64.StdEncoding.EncodeToString([]byte("too short hunter2")) + "\n" + seed + "\n",
		"a seed that is too long": base64.StdEncoding.EncodeToString(make([]byte, 33)) + "\n",
	} {
		if err := os.WriteFile(m.keyPath(), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		first, _, _ := strings.Cut(content, "\n")
		for command, args := range commands {
			out, err := m.run(args...)
			if err == nil || !strings.Contains(out, "machine.key") || strings.Contains(out, "hunter2") || first != "" && strings.Contains(out, first) || strings.Contains(out, seed) {
				t.Errorf("%s with %s: %v %q", command, label, err, safe(string(out)))
			}
		}
	}
}

func TestTheCreatedDateOfAMachineIsTheUTCDateWhateverTheLocalTimeZoneOfTheSwitchboardAndTheCommand(t *testing.T) {
	t.Parallel()
	for _, zone := range []string{"Pacific/Kiritimati", "Etc/GMT+12"} {
		zone := zone
		t.Run(zone, func(t *testing.T) {
			t.Parallel()
			h := startHostWithEnv(t, []string{"TZ=" + zone})
			before := time.Now().UTC().Format("2006-01-02")
			h.machine("linux-box")
			after := time.Now().UTC().Format("2006-01-02")
			listed := strings.Split(strings.TrimSuffix(listedMachines(t, h), "\n"), "\t")
			if len(listed) != 3 || listed[0] != "linux-box" || listed[2] != before && listed[2] != after {
				t.Errorf("under TZ=%s machines printed %q, want the UTC date %s or %s in the third field", zone, listed, before, after)
			}
			entries := storedEntries(t, readFileOrFail(t, h.dataFile("machines.json")))
			if len(entries) != 1 {
				t.Fatalf("machines.json holds %d machines", len(entries))
			}
			stamp, _ := entries[0]["created"].(string)
			if !strings.HasSuffix(stamp, "Z") || len(stamp) < 10 || stamp[:10] != before && stamp[:10] != after {
				t.Errorf("under TZ=%s the created field is %q, want an RFC 3339 UTC time on %s or %s", zone, stamp, before, after)
			}
		})
	}
}

func TestAValidCodeWithAValid32ByteKeyFollowedByExtraCharactersAnswers400AndIsNotUsedUp(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	code := h.invite("linux-box")
	failRedeems(t, h, 4)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	clean := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	for _, trailing := range []string{"!!", "!", "!!!!"} {
		body, _ := json.Marshal(map[string]string{"code": code, "public_key": clean + trailing})
		if status, _ := h.rawRequest("PATCH", "/v1/machines", body, nil); status != http.StatusBadRequest {
			t.Errorf("a valid key followed by %q answered %d, want 400", trailing, status)
		}
	}
	body, _ := json.Marshal(map[string]string{"code": code, "public_key": clean})
	if status, answer := h.rawRequest("PATCH", "/v1/machines", body, nil); status < 200 || status > 299 || !strings.Contains(answer, "linux-box") {
		t.Errorf("the redeem with a clean key after the refused ones answered %d %s", status, h.clean(answer))
	}
	if !strings.Contains(listedMachines(t, h), "linux-box") {
		t.Error("the machine was not made")
	}
}

func TestJoinWithASecretIsRefusedAndChangesNoFileWhileAMachineKeyIsSaved(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.machine("linux-box")
	m.registerAgent("alpha")
	before := filesUnder(t, m.home)
	withSecret := append(append([]string{}, m.env...), "LANDLINE_SECRET="+h.secret)
	for label, env := range map[string][]string{"with the secret in the environment": withSecret, "with no secret": m.env} {
		out, err := runIn(t, h.bin, env, m.home, "server", "join", h.addr)
		if err == nil {
			t.Errorf("a join with a secret %s succeeded: %s", label, h.clean(out))
		}
		if strings.Contains(out, h.secret) {
			t.Errorf("a refused join %s printed the secret", label)
		}
		if changed := changedFiles(before, filesUnder(t, m.home)); len(changed) != 0 {
			t.Errorf("a refused join %s changed %v", label, changed)
		}
	}
	if out, err := m.run("agents", "--json"); err != nil {
		t.Errorf("the machine stopped working after a refused join: %v %s", err, safe(string(out)))
	}
}

func TestJoinWithAnEmptyCodeIsAnErrorThatNeverAsksForASecretAndChangesNoFile(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	m := h.newMachineHome()
	writeConfigFile(t, m.home, "config.toml", "secret = \""+joinedSecret+joinedSecret+"\"\nhost = \"old.invalid\"\nport = 1\n")
	before := filesUnder(t, m.home)
	withSecret := append(append([]string{}, m.env...), "LANDLINE_SECRET="+h.secret)
	for label, env := range map[string][]string{"with the secret in the environment": withSecret, "with no secret": m.env} {
		for _, args := range [][]string{{"server", "join", h.addr, "--code="}, {"server", "join", h.addr, "--code", ""}} {
			out, err := runIn(t, h.bin, env, m.home, args...)
			if err == nil {
				t.Errorf("%q %s succeeded: %s", args[3:], label, h.clean(out))
			}
			if strings.Contains(strings.ToLower(out), "secret:") || strings.Contains(out, h.secret) {
				t.Errorf("%q %s asked for or printed a secret: %q", args[3:], label, h.clean(out))
			}
			if changed := changedFiles(before, filesUnder(t, m.home)); len(changed) != 0 {
				t.Errorf("%q %s changed %v", args[3:], label, changed)
			}
		}
	}
}

func TestInviteRunWithALandlineURLThatNamesAnotherLocalPortOfTheSwitchboardPrintsAJoinLineWithThatPort(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	proxy := startProxy(t, h.addr)
	if proxy.port == h.port {
		t.Fatal("the proxy listens on the switchboard's own port")
	}
	env := append(append([]string{}, h.env...), "LANDLINE_URL=http://"+net.JoinHostPort(proxy.host, proxy.port))
	out, err := runIn(t, h.bin, env, h.home, "server", "invite", "--name", "linux-box")
	if err != nil {
		t.Fatalf("invite through the other port: %v %s", err, h.clean(out))
	}
	found := joinCommandLine.FindStringSubmatch(strings.TrimSpace(out))
	if found == nil {
		t.Fatalf("invite printed %q", h.clean(out))
	}
	h.issued = append(h.issued, found[2])
	if _, port, splitErr := net.SplitHostPort(found[1]); splitErr != nil || port != proxy.port {
		t.Errorf("the join line names %q, want port %s, the port LANDLINE_URL named and not the saved port %s", found[1], proxy.port, h.port)
	}
	if len(proxy.requests()) == 0 {
		t.Error("the invite did not go through the port that LANDLINE_URL named")
	}
	if joinOut, joinErr := h.newMachineHome().run("server", "join", found[1], "--code", found[2]); joinErr != nil {
		t.Errorf("the printed join command did not work: %v %s", joinErr, h.clean(joinOut))
	}
}

func TestAMalformedMachineKeyDoesNotStopMachinesInviteOrRevokeOnTheSwitchboardMachineThatHoldsTheSharedSecret(t *testing.T) {
	t.Parallel()
	h := startHost(t)
	h.machine("linux-box")
	for label, content := range map[string]string{
		"not base64":   "this-is-not-base64-content-hunter2!!\n",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("too short hunter2")) + "\n",
		"empty":        "\n",
	} {
		writeConfigFile(t, h.home, "machine.key", content)
		if out, err := h.run("server", "machines"); err != nil || !strings.Contains(out, "linux-box") {
			t.Errorf("machines with a key file that is %s: %v %q", label, err, safe(out))
		}
		if out, err := h.run("server", "invite", "--name", "probe-box"); err != nil || !joinCommandLine.MatchString(strings.TrimSpace(out)) {
			t.Errorf("invite with a key file that is %s: %v %q", label, err, safe(out))
		}
		if out, err := h.run("server", "revoke", "probe-box"); err != nil {
			t.Errorf("revoke of a pending invite with a key file that is %s: %v %q", label, err, safe(out))
		}
	}
	if out, err := h.run("server", "revoke", "linux-box"); err != nil {
		t.Errorf("revoke of a machine with a malformed key file: %v %q", err, safe(out))
	}
	if out, err := h.run("server", "revoke", "--all"); err != nil {
		t.Errorf("revoke --all with a malformed key file: %v %q", err, safe(out))
	}
	if twelveHex.MatchString(listedMachines(t, h)) {
		t.Error("a machine remains after revoke")
	}
}
