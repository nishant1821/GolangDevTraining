package user

import "time"

// User — ye tera DB table ka "shape" hai.
// Node/Sequelize: sequelize.define("User", { email: {...}, password: {...} })
// Python/SQLAlchemy: class User(Base): __tablename__ = "users" ...
//
// struct tags (backticks wale) do jagah kaam karte hain ek saath:
//
//	`gorm:"..."` → GORM ko batata hai DB column kaisa banega (constraint, index, etc.)
//	`json:"..."` → encoding/json ko batata hai response mein field ka naam/behaviour kya ho
type User struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Email string `gorm:"uniqueIndex;not null" json:"email"`

	// YOUR TURN: Password field khud likh.
	//   - gorm tag: "not null" laga (khali password allowed nahi hona chahiye)
	//   - json tag: is field ko response mein KABHI mat bhejo
	//     hint: ek special json tag value hoti hai jo field ko poori tarah skip kar deti hai
	//   soch (self-check): agar tu json:"-" na lagaye, aur /auth/register handler seedha
	//   `json.NewEncoder(w).Encode(user)` kar de, toh response body mein exactly kya leak hoga?
	Password string `gorm:"not null" json:"-"`

	CreatedAt time.Time `json:"created_at"`
}
