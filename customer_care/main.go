package main

import (
	"errors"
	"fmt"
	"sort"
)

// ---------- Models ----------

type IssueType string

const (
	PaymentRelated    IssueType = "PAYMENT_RELATED"
	MutualFundRelated IssueType = "MUTUAL_FUND_RELATED"
	GoldRelated       IssueType = "GOLD_RELATED"
	InsuranceRelated  IssueType = "INSURANCE_RELATED"
)

type IssueStatus string

const (
	Open       IssueStatus = "OPEN"
	InProgress IssueStatus = "IN_PROGRESS"
	Waiting    IssueStatus = "WAITING"
	Resolved   IssueStatus = "RESOLVED"
)

type Agent struct {
	ID              string
	Name            string
	Email           string
	Expertise       map[IssueType]bool
	AssignedIssueID string
	Waitlist        []string
	History         []string
}

type Issue struct {
	ID              string
	TransactionID   string
	Type            IssueType
	Subject         string
	Description     string
	Email           string
	Status          IssueStatus
	Resolution      string
	AssignedAgentID string
}

// ---------- Repos ----------

type AgentRepo struct{ agents map[string]*Agent }

func NewAgentRepo() *AgentRepo { return &AgentRepo{agents: map[string]*Agent{}} }

func (r *AgentRepo) Save(a *Agent) { r.agents[a.ID] = a }

func (r *AgentRepo) GetByID(id string) (*Agent, error) {
	a, ok := r.agents[id]
	if !ok {
		return nil, errors.New("agent not found: " + id)
	}
	return a, nil
}

func (r *AgentRepo) GetAll() []*Agent {
	out := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type IssueRepo struct{ issues map[string]*Issue }

func NewIssueRepo() *IssueRepo { return &IssueRepo{issues: map[string]*Issue{}} }

func (r *IssueRepo) Save(i *Issue) { r.issues[i.ID] = i }

func (r *IssueRepo) GetByID(id string) (*Issue, error) {
	i, ok := r.issues[id]
	if !ok {
		return nil, errors.New("issue not found: " + id)
	}
	return i, nil
}

func (r *IssueRepo) GetAll() []*Issue {
	out := make([]*Issue, 0, len(r.issues))
	for _, i := range r.issues {
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

// ---------- Assignment strategy ----------

type AssignmentStrategy interface {
	AssignIssue(agents []*Agent, issue *Issue) (*Agent, error)
}

// DefaultStrategy: among agents with matching expertise, prefer a free one,
// otherwise the one with the shortest waitlist.
type DefaultStrategy struct{}

func (DefaultStrategy) AssignIssue(agents []*Agent, issue *Issue) (*Agent, error) {
	var best *Agent
	for _, a := range agents {
		if !a.Expertise[issue.Type] {
			continue
		}
		if a.AssignedIssueID == "" {
			return a, nil
		}
		if best == nil || len(a.Waitlist) < len(best.Waitlist) {
			best = a
		}
	}
	if best == nil {
		return nil, errors.New("no agent with expertise for " + string(issue.Type))
	}
	return best, nil
}

// ---------- Services ----------

type AgentService struct {
	agentRepo *AgentRepo
	seq       int
}

func NewAgentService(r *AgentRepo) *AgentService { return &AgentService{agentRepo: r} }

func (s *AgentService) AddAgent(email, name string, expertise []IssueType) (*Agent, error) {
	if email == "" || name == "" || len(expertise) == 0 {
		return nil, errors.New("email, name and at least one expertise are required")
	}
	for _, a := range s.agentRepo.GetAll() {
		if a.Email == email {
			return nil, errors.New("agent already exists: " + email)
		}
	}
	s.seq++
	exp := map[IssueType]bool{}
	for _, e := range expertise {
		exp[e] = true
	}
	agent := &Agent{ID: fmt.Sprintf("A%03d", s.seq), Name: name, Email: email, Expertise: exp}
	s.agentRepo.Save(agent)
	return agent, nil
}

func (s *AgentService) ViewAgentsWorkHistory() {
	for _, a := range s.agentRepo.GetAll() {
		fmt.Printf("%s (%s): %v\n", a.Name, a.ID, a.History)
	}
}

type IssueService struct {
	issueRepo *IssueRepo
	agentRepo *AgentRepo
	seq       int
}

func NewIssueService(ir *IssueRepo, ar *AgentRepo) *IssueService {
	return &IssueService{issueRepo: ir, agentRepo: ar}
}

func (s *IssueService) CreateIssue(txnID string, t IssueType, subject, desc, email string) *Issue {
	s.seq++
	issue := &Issue{
		ID: fmt.Sprintf("I%03d", s.seq), TransactionID: txnID, Type: t,
		Subject: subject, Description: desc, Email: email, Status: Open,
	}
	s.issueRepo.Save(issue)
	return issue
}

func (s *IssueService) GetIssues(filter func(*Issue) bool) []*Issue {
	var out []*Issue
	for _, i := range s.issueRepo.GetAll() {
		if filter == nil || filter(i) {
			out = append(out, i)
		}
	}
	return out
}

func (s *IssueService) UpdateIssue(id string, status IssueStatus, resolution string) error {
	issue, err := s.issueRepo.GetByID(id)
	if err != nil {
		return err
	}
	issue.Status = status
	issue.Resolution = resolution
	return nil
}

// ResolveIssue closes the issue, records it in the agent's history,
// and moves the agent on to the next waitlisted issue (if any).
func (s *IssueService) ResolveIssue(id, resolution string) error {
	issue, err := s.issueRepo.GetByID(id)
	if err != nil {
		return err
	}
	issue.Status = Resolved
	issue.Resolution = resolution

	agent, err := s.agentRepo.GetByID(issue.AssignedAgentID)
	if err != nil {
		return err
	}
	agent.History = append(agent.History, issue.ID)
	agent.AssignedIssueID = ""

	if len(agent.Waitlist) > 0 {
		nextID := agent.Waitlist[0]
		agent.Waitlist = agent.Waitlist[1:]
		next, err := s.issueRepo.GetByID(nextID)
		if err != nil {
			return err
		}
		next.Status = InProgress
		agent.AssignedIssueID = next.ID
	}
	return nil
}

type AssignmentService struct {
	issueRepo *IssueRepo
	agentRepo *AgentRepo
	strategy  AssignmentStrategy
}

func NewAssignmentService(ir *IssueRepo, ar *AgentRepo, st AssignmentStrategy) *AssignmentService {
	return &AssignmentService{issueRepo: ir, agentRepo: ar, strategy: st}
}

func (s *AssignmentService) AssignIssue(issueID string) error {
	issue, err := s.issueRepo.GetByID(issueID)
	if err != nil {
		return err
	}
	agent, err := s.strategy.AssignIssue(s.agentRepo.GetAll(), issue)
	if err != nil {
		return err
	}
	issue.AssignedAgentID = agent.ID
	if agent.AssignedIssueID == "" {
		agent.AssignedIssueID = issue.ID
		issue.Status = InProgress
	} else {
		agent.Waitlist = append(agent.Waitlist, issue.ID)
		issue.Status = Waiting
	}
	return nil
}

// ---------- Demo ----------

func main() {
	agentRepo, issueRepo := NewAgentRepo(), NewIssueRepo()
	agentSvc := NewAgentService(agentRepo)
	issueSvc := NewIssueService(issueRepo, agentRepo)
	assignSvc := NewAssignmentService(issueRepo, agentRepo, DefaultStrategy{})

	agentSvc.AddAgent("a1@x.com", "Asha", []IssueType{PaymentRelated, GoldRelated})
	agentSvc.AddAgent("b2@x.com", "Ben", []IssueType{PaymentRelated})

	i1 := issueSvc.CreateIssue("T1", PaymentRelated, "Failed payment", "Money debited", "u1@x.com")
	i2 := issueSvc.CreateIssue("T2", PaymentRelated, "Double charge", "Charged twice", "u2@x.com")
	i3 := issueSvc.CreateIssue("T3", PaymentRelated, "Refund delay", "No refund yet", "u3@x.com")

	for _, i := range []*Issue{i1, i2, i3} {
		if err := assignSvc.AssignIssue(i.ID); err != nil {
			fmt.Println("assign error:", err)
		}
	}
	for _, i := range issueSvc.GetIssues(nil) {
		fmt.Printf("%s -> %s [%s]\n", i.ID, i.AssignedAgentID, i.Status)
	}

	issueSvc.ResolveIssue(i1.ID, "Refunded")
	issueSvc.ResolveIssue(i3.ID, "Refund processed")

	fmt.Println("--- history ---")
	agentSvc.ViewAgentsWorkHistory()
}