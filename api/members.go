package api

import "fmt"

// ========== Project Members & Invitations ==========

type ProjectMember struct {
	ProjectID string  `json:"project_id"`
	UserID    string  `json:"user_id"`
	Role      string  `json:"role"`
	InvitedBy *string `json:"invited_by,omitempty"`
	CreatedAt string  `json:"created_at"`
	UserEmail string  `json:"user_email,omitempty"`
	UserName  string  `json:"user_name,omitempty"`
}

type ProjectInvitation struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	Token       string  `json:"token,omitempty"`
	InvitedBy   string  `json:"invited_by"`
	AcceptedAt  *string `json:"accepted_at,omitempty"`
	ExpiresAt   string  `json:"expires_at"`
	CreatedAt   string  `json:"created_at"`
	ProjectName string  `json:"project_name,omitempty"`
}

type InviteProjectMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type UpdateProjectMemberRoleRequest struct {
	Role string `json:"role"`
}

type ProjectInvitationInfo struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	ExpiresAt   string `json:"expires_at"`
}

type ProjectRoleInfo struct {
	Role       string           `json:"role"`
	Restricted bool             `json:"restricted,omitempty"`
	Resources  []MemberResource `json:"resources,omitempty"`
}

type MemberResource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (c *Client) ListProjectMembers(projectID string) ([]ProjectMember, error) {
	var members []ProjectMember
	err := c.Get(fmt.Sprintf("/api/projects/%s/members", projectID), &members)
	return members, err
}

func (c *Client) UpdateProjectMemberRole(projectID, userID, role string) error {
	return c.Patch(fmt.Sprintf("/api/projects/%s/members/%s", projectID, userID),
		UpdateProjectMemberRoleRequest{Role: role}, nil)
}

func (c *Client) RemoveProjectMember(projectID, userID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/members/%s", projectID, userID), nil)
}

func (c *Client) ListProjectInvitations(projectID string) ([]ProjectInvitation, error) {
	var invs []ProjectInvitation
	err := c.Get(fmt.Sprintf("/api/projects/%s/invitations", projectID), &invs)
	return invs, err
}

func (c *Client) CreateProjectInvitation(projectID, email, role string) (*ProjectInvitation, error) {
	var inv ProjectInvitation
	err := c.Post(fmt.Sprintf("/api/projects/%s/invitations", projectID),
		InviteProjectMemberRequest{Email: email, Role: role}, &inv)
	return &inv, err
}

func (c *Client) RevokeProjectInvitation(projectID, inviteID string) error {
	return c.Delete(fmt.Sprintf("/api/projects/%s/invitations/%s", projectID, inviteID), nil)
}

func (c *Client) GetMyProjectRole(projectID string) (*ProjectRoleInfo, error) {
	var info ProjectRoleInfo
	err := c.Get(fmt.Sprintf("/api/projects/%s/my-role", projectID), &info)
	return &info, err
}

func (c *Client) GetMemberResources(projectID, userID string) ([]MemberResource, error) {
	var resp struct {
		Resources []MemberResource `json:"resources"`
	}
	err := c.Get(fmt.Sprintf("/api/projects/%s/members/%s/resources", projectID, userID), &resp)
	return resp.Resources, err
}

func (c *Client) PutMemberResources(projectID, userID string, resources []MemberResource) error {
	body := map[string]interface{}{"resources": resources}
	return c.Put(fmt.Sprintf("/api/projects/%s/members/%s/resources", projectID, userID), body, nil)
}

func (c *Client) GetProjectInvitationInfo(token string) (*ProjectInvitationInfo, error) {
	var info ProjectInvitationInfo
	err := c.Get(fmt.Sprintf("/api/project-invitations/%s", token), &info)
	return &info, err
}

func (c *Client) AcceptProjectInvitation(token string) (*ProjectInvitationInfo, error) {
	var info ProjectInvitationInfo
	err := c.Post(fmt.Sprintf("/api/project-invitations/%s/accept", token), nil, &info)
	return &info, err
}
