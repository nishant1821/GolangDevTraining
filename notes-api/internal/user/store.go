package user

import "gorm.io/gorm"

// Store — DB ke saath seedha baat karne wali layer (business logic yahan nahi, wo service/handler mein).
// Node: Mongoose model methods jaisa — User.create(), User.findOne({ where: { email } })
// Python: SQLAlchemy — session.query(User).filter_by(email=...).first()
type Store struct {
	db *gorm.DB
}

// NewStore — constructor. db upar se inject hota hai (urlshortner wale store/service/handler
// injection pattern jaisa hi) taaki tests mein fake DB pass kar sakein.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create — naya user row insert karta hai.
// Node: await User.create({ email, password })
func (s *Store) Create(u *User) error {
	return s.db.Create(u).Error
}

// YOUR TURN: FindByEmail likh — login ke time email se user dhoondhna hai.
// Node: await User.findOne({ where: { email } })
//
// hint:
//
//	var u User
//	err := s.db.Where("email = ?", email).First(&u).Error
//	return &u, err
//
// self-check: agar email match na ho, GORM `gorm.ErrRecordNotFound` return karta hai
// (Node mein findOne null return karta hai — Go mein ye ek explicit error hota hai,
// isliye handler ko error check karna hi padega, chup-chaap nil nahi milega).
func (s *Store) FindByEmail(email string) (*User, error) {

	var u User
	query := s.db.Where("email = ?", email)

	// Step 2 query run karo and result me daaldo
	result := query.First(&u)

	err := result.Error

	if err != nil {
		return nil, err

	}

	return &u, nil
}
