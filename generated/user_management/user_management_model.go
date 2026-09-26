package user_management

type User struct {
	ID       int64
	Username string
	Email    string
}

type Userdetails struct {
	ID     int64
	UserID int64
	Phone1 int64
	Phone2 *string
}
