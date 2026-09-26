package voting

type VotingEvent struct {
	ID          int64
	Title       string
	Description *string
	OpensAt     string
	ClosesAt    string
}

type Vote struct {
	ID            int64
	VotingEventID int64
	VoterUserId   int64
	Choice        string
}
