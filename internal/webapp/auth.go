package webapp

import (
	"context"
	"net/http"
	"strings"

	"kube-env/internal/repository"
)

type principalKey struct{}

type Principal struct {
	User     string
	Email    string
	IsAdmin  bool
	AllEnv   bool
	AllRole  string
	EnvRoles map[string]string
}

func normalizeTrustedProxyAuth(opts TrustedProxyAuthOptions) TrustedProxyAuthOptions {
	if opts.HeaderUser == "" {
		opts.HeaderUser = "X-Auth-User"
	}
	if opts.HeaderEmail == "" {
		opts.HeaderEmail = "X-Auth-Email"
	}
	if opts.HeaderGroups == "" {
		opts.HeaderGroups = "X-Auth-Groups"
	}
	if opts.GroupPrefix == "" {
		opts.GroupPrefix = "kube-edit-app"
	}
	return opts
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if !s.options.TrustedProxyAuth.Enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		cfg := s.options.TrustedProxyAuth
		user := strings.TrimSpace(r.Header.Get(cfg.HeaderUser))
		if user == "" {
			writeError(w, http.StatusUnauthorized, "missing trusted proxy identity")
			return
		}
		p := parseTrustedProxyGroups(r.Header.Get(cfg.HeaderGroups), cfg.GroupPrefix)
		p.User = user
		p.Email = strings.TrimSpace(r.Header.Get(cfg.HeaderEmail))
		if !s.authorizeRequest(r, p) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

func isPublicPath(path string) bool {
	return path == "/healthz" || strings.HasPrefix(path, "/static/")
}

func parseTrustedProxyGroups(groupsHeader, prefix string) *Principal {
	p := &Principal{EnvRoles: map[string]string{}}
	if prefix == "" {
		prefix = "kube-edit-app"
	}
	for _, group := range strings.Split(groupsHeader, ",") {
		group = strings.TrimSpace(group)
		if !strings.HasPrefix(group, prefix+":") {
			continue
		}
		rest := strings.TrimPrefix(group, prefix+":")
		if rest == "role:admin" {
			p.IsAdmin = true
			continue
		}
		parts := strings.SplitN(rest, ":", 3)
		if len(parts) != 3 || parts[0] != "env" {
			continue
		}
		env, role := parts[1], parts[2]
		if role != "reader" && role != "writer" {
			continue
		}
		if env == "*" {
			p.AllEnv = true
			if roleRank(role) > roleRank(p.AllRole) {
				p.AllRole = role
			}
			continue
		}
		if roleRank(role) > roleRank(p.EnvRoles[env]) {
			p.EnvRoles[env] = role
		}
	}
	return p
}

func roleRank(role string) int {
	switch role {
	case "writer":
		return 2
	case "reader":
		return 1
	default:
		return 0
	}
}

func (p *Principal) CanReadEnv(env string) bool {
	if p == nil || env == "" {
		return false
	}
	return p.IsAdmin || p.AllEnv || p.EnvRoles[env] != ""
}

func (p *Principal) CanWriteEnv(env string) bool {
	if p == nil || env == "" {
		return false
	}
	return p.IsAdmin || (p.AllEnv && p.AllRole == "writer") || p.EnvRoles[env] == "writer"
}

func (s *Server) authorizeRequest(r *http.Request, p *Principal) bool {
	path := r.URL.Path
	if path == "/" || path == "/api/v1/info" {
		return true
	}
	if path == "/api/v1/git/status" || strings.HasPrefix(path, "/api/v1/git/diff/") {
		return p.IsAdmin || p.AllEnv || len(p.EnvRoles) > 0
	}
	if path == "/api/v1/git/commit" || strings.HasPrefix(path, "/api/v1/git/restore/") {
		return p.IsAdmin
	}
	if path == "/api/v1/envs" {
		return p.IsAdmin || p.AllEnv || len(p.EnvRoles) > 0
	}
	if strings.HasPrefix(path, "/api/v1/envs/") {
		env, ok := envFromAPIPath(path)
		if !ok {
			return false
		}
		if isEnvWriteRequest(r.Method, path) {
			return p.CanWriteEnv(env)
		}
		return p.CanReadEnv(env)
	}
	return false
}

func envFromAPIPath(path string) (string, bool) {
	rest := strings.TrimPrefix(path, "/api/v1/envs/")
	if rest == path || rest == "" {
		return "", false
	}
	env, _, _ := strings.Cut(rest, "/")
	return env, env != ""
}

func isEnvWriteRequest(method, path string) bool {
	if method == http.MethodGet {
		return false
	}
	switch {
	case strings.HasSuffix(path, "/validate"),
		strings.HasSuffix(path, "/summary"),
		strings.HasSuffix(path, "/inventory"),
		strings.Contains(path, "/preview/"):
		return false
	case strings.HasSuffix(path, "/preview"):
		return false
	default:
		return true
	}
}

func (s *Server) filterEnvironmentsForRequest(r *http.Request, envs []repository.Environment) []repository.Environment {
	if !s.options.TrustedProxyAuth.Enabled {
		return envs
	}
	p, _ := r.Context().Value(principalKey{}).(*Principal)
	if p == nil {
		return nil
	}
	if p.IsAdmin || p.AllEnv {
		return envs
	}
	out := envs[:0:0]
	for _, env := range envs {
		if p.CanReadEnv(env.Name) {
			out = append(out, env)
		}
	}
	return out
}
