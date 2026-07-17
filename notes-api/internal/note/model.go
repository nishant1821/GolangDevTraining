package note

import "time"

// Note — har row ek user ka ek note hai.
// Node/Sequelize: Note.belongsTo(User) wala relation is UserID field se banta hai
// Python/SQLAlchemy: user_id = Column(Integer, ForeignKey("users.id"))
type Note struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Title string `gorm:"not null" json:"title"`
	Body  string `json:"body"`

	// YOUR TURN: UserID field — is poore project ka sabse important field.
	// Ownership isi pe based hai: har query "WHERE user_id = ?" karke filter karegi
	// taaki koi user dusre ka note na dekh paaye.
	//   - type: uint (User.ID jaisa hi type match hona chahiye)
	//   - gorm tag: index laga do — kyunki har query is column pe filter karegi,
	//     bina index ke table badi hote hi slow ho jaayegi
	//   - json tag: normal rakh sakta hai, ye sensitive nahi hai
	UserID uint `gorm:"index;not null" json:"user_id"`

	CreatedAt time.Time `json:"created_at"`
}
