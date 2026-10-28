package http

import (
	"net/http"

	"github.com/lapp-coder/task-manager-service/internal/domain"
)

func (h *handler) createTeam(w http.ResponseWriter, r *http.Request) error {
	var req createTeamRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	t, err := h.teams.Create(r.Context(), actorID(r.Context()), req.Name)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, teamResponseOf(t, domain.RoleOwner))
	return nil
}

func (h *handler) listTeams(w http.ResponseWriter, r *http.Request) error {
	memberships, err := h.teams.List(r.Context(), actorID(r.Context()))
	if err != nil {
		return err
	}
	resp := make([]teamResponse, len(memberships))
	for i, m := range memberships {
		resp[i] = teamResponseOf(m.Team, m.Role)
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *handler) listMembers(w http.ResponseWriter, r *http.Request) error {
	teamID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	members, err := h.teams.Members(r.Context(), actorID(r.Context()), teamID)
	if err != nil {
		return err
	}
	resp := make([]memberResponse, len(members))
	for i, m := range members {
		resp[i] = memberResponseOf(m)
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *handler) invite(w http.ResponseWriter, r *http.Request) error {
	teamID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var req inviteRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	if req.Role == "" {
		req.Role = string(domain.RoleMember)
	}
	role, err := domain.ParseInvitableRole(req.Role)
	if err != nil {
		return err
	}
	m, err := h.teams.Invite(r.Context(), actorID(r.Context()), teamID, req.Email, role)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, memberResponseOf(m))
	return nil
}

func (h *handler) changeRole(w http.ResponseWriter, r *http.Request) error {
	teamID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	userID, err := pathID(r, "user_id")
	if err != nil {
		return err
	}
	var req changeRoleRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	role, err := domain.ParseInvitableRole(req.Role)
	if err != nil {
		return err
	}
	m, err := h.teams.ChangeRole(r.Context(), actorID(r.Context()), teamID, userID, role)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, memberResponseOf(domain.TeamMember{Member: m}))
	return nil
}

func (h *handler) teamStats(w http.ResponseWriter, r *http.Request) error {
	teamID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	stats, err := h.teams.Stats(r.Context(), actorID(r.Context()), teamID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, statsResponseOf(stats))
	return nil
}
