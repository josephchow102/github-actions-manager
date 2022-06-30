package controller

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oursky/github-actions-manager/pkg/utils/httputil"

	"go.uber.org/zap"
)

type AgentResponse struct {
	Agent     Agent    `json:"agent"`
	TargetURL string   `json:"targetURL"`
	Token     string   `json:"token"`
	Group     string   `json:"group"`
	Labels    []string `json:"labels"`
}

func (s *server) apiAgentGetDelete(rw http.ResponseWriter, r *http.Request) {
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/agent/"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
	}

	switch r.Method {
	case http.MethodGet:
		s.apiAgentGet(rw, r, id)
	case http.MethodDelete:
		s.apiAgentDelete(rw, r, id)
	default:
		http.Error(rw, "unsupported method", http.StatusBadRequest)
	}
}

func (s *server) apiAgentGet(rw http.ResponseWriter, r *http.Request, id string) {
	agent, err := s.provider.State().GetAgent(id)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	} else if agent == nil {
		http.NotFound(rw, r)
		return
	}

	httputil.RespondJSON(rw, agent)
}

func (s *server) apiAgentDelete(rw http.ResponseWriter, r *http.Request, id string) {
	agent, err := s.provider.State().GetAgent(id)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	} else if agent == nil {
		http.NotFound(rw, r)
		return
	}

	if agent.State != AgentStateTerminating {
		s.logger.Info("requested agent termination", zap.String("id", agent.ID))
		err = s.provider.State().UpdateAgent(agent.ID, func(a *Agent) {
			a.State = AgentStateTerminating
			a.LastTransitionTime = time.Now()
		})
		if err != nil {
			s.logger.Error("failed to terminate agent", zap.Error(err), zap.String("id", agent.ID))
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	rw.WriteHeader(200)
}

func (s *server) apiAgentPost(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "unsupported method", http.StatusBadRequest)
		return
	}

	runnerName := r.FormValue("runnerName")

	regToken, targetURL, err := s.managerAPI.GetRegistrationToken(r.Context())
	if err != nil {
		s.logger.Error("cannot get registration token", zap.Error(err))
		http.Error(rw, "cannot get registration token", http.StatusInternalServerError)
		return
	}

	resp, err := s.provider.RegisterAgent(r, runnerName, regToken, targetURL)
	if err != nil {
		s.logger.Error("cannot register agent", zap.Error(err), zap.String("runnerName", runnerName))
		http.Error(rw, "cannot register agent", http.StatusInternalServerError)
		return
	}
	httputil.RespondJSON(rw, resp)
}
