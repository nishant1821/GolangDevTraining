package note

import "gorm.io/gorm"

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create — naya note insert karta hai. UserID handler se already set hoke aayega (Step 10 mein).
func (s *Store) Create(n *Note) error {
	return s.db.Create(n).Error
}

// ListByUser — sirf usi user ke notes, kabhi saari table nahi.
// Node: await Note.findAll({ where: { userId } })
func (s *Store) ListByUser(userID uint) ([]Note, error) {
	var notes []Note
	err := s.db.Where("user_id = ?", userID).Find(&notes).Error
	return notes, err
}

// YOUR TURN: GetByID likh — GET /notes/{id} isko use karega.
// Node: await Note.findOne({ where: { id, userId } })
//
// IMPORTANT: query mein hi dono condition daal — "id = ? AND user_id = ?".
// Alag se "id se dhoondo, phir userID compare karo" mat karna — query mein hi ownership
// bake kar dena best practice hai (ek hi DB round-trip, aur galti se check bhoolne ka chance nahi).
//
// hint:
//
//	var n Note
//	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&n).Error
//	return &n, err
//
// self-check: agar note ID exist karta hai DB mein but kisi AUR user ka hai, ye query kya return karegi?
// gorm.ErrRecordNotFound hi aayega — jaise note exist hi nahi karta. Yehi wo "404, not 403" wala
// interview trick hai: attacker ko pata bhi nahi chalna chahiye ki ID valid hai, bas kisi aur ki hai.
func (s *Store) GetByID(id, userID uint) (*Note, error) {
	// TODO
	var n Note
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&n).Error

	return &n, err
}

// YOUR TURN: Update likh — same ownership-scoped pattern.
// hint: s.db.Model(&Note{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{...})
// ya pehle GetByID call karke fields update kar aur s.db.Save(&n) — dono valid approach hain
func (s *Store) Update(id, userID uint, title, body string) error {
	// TODO
	result := s.db.Model(&Note{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(Note{Title: title, Body: body})
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

// YOUR TURN: Delete likh — same ownership-scoped pattern.
// hint: s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Note{}).Error
// self-check: GORM ka .Delete() agar 0 rows match kare (galat owner ya galat ID), error dega ya nahi?
// (nahi dega! isliye handler mein row-count check karna padega agar "not found" bolna hai —
//
//	RowsAffected field available hota hai result object pe)
func (s *Store) Delete(id, userID uint) error {
	result := s.db.Model(&Note{}).Where("id = ? AND user_id = ?", id, userID).Delete(&Note{})

	if result.Error != nil {
		return result.Error // ← ye reh gaya tha
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
