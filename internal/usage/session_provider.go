package usage

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/yetone/magpie/internal/filememo"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
)

type desktopSessionInfo struct {
	Session string `json:"cliSessionId"`
	Email   string `json:"emailAddress"`
}

type desktopSessionIdentity struct {
	account, org, email string
	linkable            bool
}

// sessionResolver only identifies accounts recorded by local sessions. It does
// not infer a service provider, endpoint, or billing account.
type sessionResolver struct {
	identities []provider.SessionIdentity
	desktop    map[string]desktopSessionIdentity // absolute metadata file
	bySession  map[string][]desktopSessionIdentity
	emails     map[string]string // exact account/organization identity
	// stated marks the pairs an address was stated for and then contradicted:
	// a metadata file naming no address never blanks one that did (noteEmail).
	stated map[string]bool
	roots  []string
}

func newSessionResolver(logs []sessions.Call) *sessionResolver {
	r := &sessionResolver{}
	if len(logs) == 0 {
		return r
	}
	r.identities = provider.SessionIdentities(sessions.CodexDir())
	r.desktop = map[string]desktopSessionIdentity{}
	r.bySession = map[string][]desktopSessionIdentity{}
	r.emails = map[string]string{}
	r.stated = map[string]bool{}
	for _, id := range r.identities {
		if id.Agent == "claude" {
			r.noteEmail(id.AccountID, id.OrganizationID, id.User)
		}
	}
	r.roots = sessions.DesktopDataDirs()
	for _, root := range r.roots {
		for _, kind := range []string{"local-agent-mode-sessions", "claude-code-sessions"} {
			files, _ := sessions.SessionGlob(filepath.Join(root, kind, "*", "*", "local_*.json"))
			for _, path := range files {
				meta, err := filememo.Read("session desktop identity", path, func(b []byte) (desktopSessionInfo, error) {
					var info desktopSessionInfo
					err := json.Unmarshal(b, &info)
					return info, err
				})
				if err != nil {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				parts := strings.Split(rel, string(filepath.Separator))
				id := desktopSessionIdentity{account: parts[1], org: parts[2], email: meta.Email,
					linkable: !strings.Contains(filepath.Base(root), "-3p") && uuidIdentity(parts[1]) && uuidIdentity(parts[2])}
				r.desktop[path] = id
				if meta.Session != "" {
					r.bySession[meta.Session] = append(r.bySession[meta.Session], id)
				}
				if id.linkable {
					r.noteEmail(id.account, id.org, id.email)
				}
			}
		}
	}
	return r
}

func uuidIdentity(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// noteEmail remembers the address an account/organization pair is known by.
//
// Three rules, each from what the desktop's own metadata does (measured
// 2026-11-19):
//
//   - A sign-in that names no organization does not share a pair with one
//     that does. ~/.claude.json's oauthAccount may carry accountUuid alone,
//     and "account/" is then a bucket of every organization-less login of
//     that account.
//   - A metadata file that states no address does not blank one that does.
//     The Code tab writes local_*.json with the session id alone, so its
//     identity shares the Cowork tab's account and organization and would
//     otherwise read as a conflicting claim.
//   - A recorded address that is already there is not a conflict at all.
//     Only a second, different address is, and that blanks the pair: two
//     signed-in accounts of one organization are never read as the one the
//     other of them proves.
//
// An address stated by a session's own metadata is also remembered apart
// (sessionEmails), so a session whose file is not on disk any more still
// names its account: before that, only an answer that already had one worked.
func (r *sessionResolver) noteEmail(account, org, email string) {
	if account == "" || org == "" || email == "" {
		return
	}
	key := account + "/" + org
	if _, ok := r.emails[key]; !ok {
		r.emails[key] = email
		r.stated[key] = true
		return
	}
	if r.emails[key] == email {
		r.stated[key] = true
		return
	}
	// a different address than the one held: the pair is not read as either
	r.emails[key] = ""
}

func (r *sessionResolver) codexUser(c sessions.Call) string {
	if c.AccountID == "" || c.UserID == "" {
		return ""
	}
	user := ""
	for _, id := range r.identities {
		if id.Agent != "codex" || id.AccountID != c.AccountID || id.UserID != c.UserID {
			continue
		}
		if user != "" && user != id.User {
			return "" // a workspace can have more than one signed-in member
		}
		user = id.User
	}
	return user
}

func (r *sessionResolver) resolve(c sessions.Call) string {
	if c.Agent == "codex" {
		return r.codexUser(c)
	}
	id, found := r.desktopIdentity(c)
	if !found {
		return ""
	}
	if id.email == "" {
		id.email = r.emails[id.account+"/"+id.org]
	}
	if id.email == "" {
		// The session's own file wrote no address and the pair holds none
		// (a second address blanked it): read the desktop's own folder for
		// one. The metadata beside each local_*.json is how a session names
		// its account without being read as "whatever is signed in today".
		id.email = r.sessionEmail(id)
	}
	return id.email
}

// sessionEmail is the address a desktop session's folder records, read from
// the metadata files beside it when the call itself carries none: the Code
// tab's calls name their session id alone, and their account is only in the
// local_*.json under the same account and organization. A file that no
// longer exists is no answer, and neither is one that names nobody: the
// caller falls back to "" as it did before.
func (r *sessionResolver) sessionEmail(id desktopSessionIdentity) string {
	for _, root := range r.roots {
		if strings.Contains(filepath.Base(root), "-3p") {
			continue // a third-party Desktop's login is not this account's
		}
		for _, kind := range []string{desktopAgentSessions, desktopCodeSessions} {
			files, _ := sessions.SessionGlob(filepath.Join(root, kind, id.account, id.org, "local_*.json"))
			for _, path := range files {
				if meta, ok := r.desktop[path]; ok && meta.email != "" {
					return meta.email
				}
			}
		}
	}
	return ""
}

// officialLogin requires explicit login metadata for the exact session
// identity. A model, a provider ID, an email alone, or another signed-in
// account never establishes it. It makes no claim about the request's route.
func (r *sessionResolver) officialLogin(c sessions.Call, account string) bool {
	if account == "" {
		return false
	}
	if c.Agent == "codex" {
		for _, id := range r.identities {
			if id.Agent == "codex" && id.OfficialLogin && c.AccountID != "" && c.UserID != "" && id.AccountID == c.AccountID && id.UserID == c.UserID && id.User == account {
				return true
			}
		}
		return false
	}
	desktop, found := r.desktopIdentity(c)
	if !found || desktop.account == "" || desktop.org == "" {
		return false
	}
	for _, id := range r.identities {
		if id.Agent == "claude" && id.OfficialLogin && id.AccountID == desktop.account && id.OrganizationID == desktop.org && id.User == account {
			return true
		}
	}
	return false
}

// desktopAgentSessions is the folder of a desktop root that holds Claude
// desktop's agent-mode sessions, each beside the metadata naming its
// account.
const desktopAgentSessions = "local-agent-mode-sessions"

// desktopCodeSessions is the folder beside it that holds the desktop's Code
// tab (its Claude Code sessions): newSessionResolver reads the same
// local_*.json under it, so a call recorded there names its account the same
// way.
const desktopCodeSessions = "claude-code-sessions"

func (r *sessionResolver) desktopIdentity(c sessions.Call) (desktopSessionIdentity, bool) {
	if c.Agent != "claude" && c.Agent != "claude-desktop" {
		return desktopSessionIdentity{}, false
	}
	var id desktopSessionIdentity
	found := false
	// Only a file under one of a desktop root's session folders can name
	// one: the rest (every Claude Code session, ~380k calls in a long
	// history, asked twice each) skip the Rel and the Split, which were a
	// fifth of what a started magpie's first Requests read allocated.
	//
	// Both folders newSessionResolver reads are counted: the Cowork tab's
	// local-agent-mode-sessions and the Code tab's claude-code-sessions. The
	// guard named the first alone, so a call the Code tab recorded was
	// resolved by its session id alone and its account and organization —
	// the only place those are written — were never read.
	roots := r.roots
	if !strings.Contains(c.File, desktopAgentSessions) && !strings.Contains(c.File, desktopCodeSessions) {
		roots = nil
	}
	for _, root := range roots {
		rel, err := filepath.Rel(root, c.File)
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 4 && (parts[0] == desktopAgentSessions || parts[0] == desktopCodeSessions) && strings.HasPrefix(parts[3], "local_") && parts[4] == ".claude" {
			path := filepath.Join(root, filepath.Join(parts[:4]...)) + ".json"
			id, found = r.desktop[path]
			break
		}
	}
	if !found {
		if ids := r.bySession[c.Session]; len(ids) == 1 {
			id, found = ids[0], true
		}
	}
	return id, found
}
