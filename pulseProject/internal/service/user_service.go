package service

// user_service.go — Stage 7.
//
// UserService handles registration and login.
// It knows:
//   - bcrypt: hashing + verifying passwords
//   - JWT: signing tokens for authenticated users
//   - UserRepository interface: create and fetch users
//
// It knows NOTHING about:
//   - HTTP (no http.Request / http.ResponseWriter)
//   - *gorm.DB — only uses the repository interface
//
// Layer diagram:
//   handler → UserService → UserRepository → DB
//
// Python analogy:
//   class UserService:
//       def __init__(self, users_repo, jwt_secret): ...
//       def register(self, email, password) -> User
//       def login(self, email, password) -> (User, token_str)
//
// Node.js analogy:
//   class UserService {
//     constructor(usersRepo, jwtSecret) { ... }
//     async register(email, password): User
//     async login(email, password): { user, token }
//   }

import (
	"context"  // context.Context — every DB call is cancellable
	"errors"   // errors.Is — sentinel-error comparison
	"fmt"      // fmt.Errorf — wrap errors with context
	"strings"  // strings.TrimSpace, strings.ToLower — sanitise input
	"time"     // time.Now, time.Duration — JWT expiry

	"github.com/golang-jwt/jwt/v5" // jwt.NewWithClaims, jwt.SigningMethodHS256
	"golang.org/x/crypto/bcrypt"   // bcrypt.GenerateFromPassword, bcrypt.CompareHashAndPassword

	"github.com/nishantks908/pulse/internal/domain"     // domain.User, domain.ErrNotFound, domain.ErrConflict
	"github.com/nishantks908/pulse/internal/repository" // repository.UserRepository
)

// ─────────────────────────────────────────────────────────────────────────────
// PulseClaims — the JWT payload
// ─────────────────────────────────────────────────────────────────────────────

// PulseClaims embeds the standard JWT claims (exp, iat, …) and adds our
// application-specific field: UserID.
//
// jwt.RegisteredClaims carries standard fields:
//   ExpiresAt (exp) — when the token expires
//   IssuedAt  (iat) — when the token was created
//   Issuer    (iss) — who issued it ("pulse")
//
// Python: { "sub": str(user_id), "exp": datetime.utcnow() + timedelta(...) }
// Node.js: jwt.sign({ userId }, secret, { expiresIn: "24h" })
// Go:      embed jwt.RegisteredClaims, add custom fields
type PulseClaims struct {
	UserID uint `json:"user_id"` // our custom claim — extracted by auth middleware
	jwt.RegisteredClaims        // standard: exp, iat, iss, sub …
}

// ─────────────────────────────────────────────────────────────────────────────
// UserService — the struct
// ─────────────────────────────────────────────────────────────────────────────

// UserService is the public interface for auth operations.
// Handlers depend on this interface so they can be tested with a mock.
//
// Python: class UserServiceProtocol(Protocol): ...
// Node.js/TypeScript: interface UserService { register(...): Promise<User>; login(...): Promise<{user, token}> }
type UserService interface {
	Register(ctx context.Context, email, password string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*domain.User, string, error)
}

// userService is the concrete implementation — unexported.
type userService struct {
	users     repository.UserRepository // create/fetch users from DB
	jwtSecret []byte                    // HMAC-SHA256 signing key (from config)
	jwtTTL    time.Duration             // how long tokens live (e.g. 24h)
}

// NewUserService is the constructor — wired in main.go.
//
// jwtSecret: raw bytes of the signing key (e.g., []byte(cfg.JWTSecret))
// jwtTTL:    token lifetime, e.g. 24*time.Hour
//
// Python: user_service = UserService(users_repo, jwt_secret, ttl)
// Node.js: const userSvc = new UserService(usersRepo, jwtSecret, ttl)
func NewUserService(
	users repository.UserRepository,
	jwtSecret []byte,
	jwtTTL time.Duration,
) UserService {
	return &userService{
		users:     users,
		jwtSecret: jwtSecret,
		jwtTTL:    jwtTTL,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Register — create a new user account
// ─────────────────────────────────────────────────────────────────────────────

// Register validates the email/password, hashes the password with bcrypt,
// and inserts the user. Returns the new User (without the hashed password
// — the Password field has json:"-" so it never appears in API responses).
//
// bcrypt cost 12: workfactor ~250ms on modern hardware.
//   Too fast (cost 4): brute-force becomes cheap.
//   Too slow (cost 15+): login feels sluggish.
//   12 is the industry default for web apps.
//
// Python: django.contrib.auth.hashers.make_password(password, hasher="bcrypt")
// Node.js: await bcrypt.hash(password, 12)
func (s *userService) Register(ctx context.Context, email, password string) (*domain.User, error) {
	// Normalise email: lowercase + trim whitespace.
	// "User@EXAMPLE.com " and "user@example.com" should be the same account.
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" {
		return nil, fmt.Errorf("register: %w: email is required", domain.ErrValidation)
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("register: %w: password must be at least 8 characters", domain.ErrValidation)
	}

	// Check for duplicate email before hashing (fast DB lookup vs slow bcrypt).
	// If email exists → domain.ErrConflict → HTTP 409.
	existing, err := s.users.ByEmail(ctx, email)
	if err == nil && existing != nil {
		// No error AND a user was returned → email already registered.
		return nil, fmt.Errorf("register: %w: email already in use", domain.ErrConflict)
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		// A real DB error (not "not found") — surface it.
		return nil, fmt.Errorf("register: check existing user: %w", err)
	}
	// If errors.Is(err, domain.ErrNotFound) → email is free, proceed.

	// Hash the password with bcrypt.
	// bcrypt.GenerateFromPassword returns a self-contained hash string:
	//   $2a$12$<22-char-salt><31-char-hash>
	// It includes the cost (12) and salt in the string itself — no need to
	// store them separately. bcrypt.CompareHashAndPassword re-extracts them.
	//
	// IMPORTANT: we NEVER store the raw password anywhere.
	//
	// Python: bcrypt.hashpw(password.encode(), bcrypt.gensalt(rounds=12))
	// Node.js: await bcrypt.hash(password, 12)
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	u := &domain.User{
		Email:    email,
		Password: string(hashed), // stored as the bcrypt hash string, never plaintext
	}

	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	return u, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Login — verify credentials and issue a JWT
// ─────────────────────────────────────────────────────────────────────────────

// Login looks up the user by email, verifies the password with bcrypt,
// and returns a signed JWT token string.
//
// Why return the token string (not a struct)?
//   The token string is the single portable credential.
//   HTTP handler puts it in the JSON body: {"token": "<string>"}
//   Client stores it and sends it as: Authorization: Bearer <string>
//
// Python:
//   user = User.objects.get(email=email)
//   check_password(password, user.password)
//   token = jwt.encode({"sub": str(user.id), "exp": ...}, SECRET, algorithm="HS256")
// Node.js:
//   const user = await User.findOne({ where: { email } })
//   await bcrypt.compare(password, user.password)
//   const token = jwt.sign({ userId: user.id }, SECRET, { expiresIn: "24h" })
func (s *userService) Login(ctx context.Context, email, password string) (*domain.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	// Fetch user by email — returns domain.ErrNotFound if not registered.
	u, err := s.users.ByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Return ErrUnauthorized, NOT "user not found".
			// Reason: if we returned "user not found", an attacker can enumerate
			// which emails are registered. ErrUnauthorized gives the same response
			// for wrong email AND wrong password — timing-safe messaging.
			return nil, "", fmt.Errorf("login: %w", domain.ErrUnauthorized)
		}
		return nil, "", fmt.Errorf("login: fetch user: %w", err)
	}

	// bcrypt.CompareHashAndPassword re-hashes the given password with the salt
	// embedded in the stored hash, then compares in constant time.
	//
	// WHY constant time?
	//   Naïve string comparison short-circuits on the first differing byte.
	//   Attackers can measure microsecond differences and use timing to brute-force.
	//   bcrypt always takes the same time regardless of where the mismatch is.
	//
	// Python: bcrypt.checkpw(password.encode(), stored_hash.encode())
	// Node.js: await bcrypt.compare(password, user.password)
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		// bcrypt returns an error for a wrong password — same response as wrong email.
		return nil, "", fmt.Errorf("login: %w", domain.ErrUnauthorized)
	}

	// Build the JWT.
	// jwt.NewWithClaims(signingMethod, claims) creates an unsigned token.
	// .SignedString(secret) signs it with HMAC-SHA256 and returns the string.
	//
	// Token format (base64url encoded):
	//   header.payload.signature
	//   e.g.: eyJhbGci...  .  eyJ1c2VyX2lk...  .  abcdef...
	now := time.Now()
	claims := PulseClaims{
		UserID: u.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "pulse",
			Subject:   fmt.Sprintf("%d", u.ID), // standard "sub" claim = user ID as string
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtTTL)),
		},
	}

	// jwt.NewWithClaims returns an *jwt.Token.
	// SignedString(key) serialises and signs it.
	//
	// Python: jwt.encode(payload, secret, algorithm="HS256")
	// Node.js: jwt.sign(payload, secret, { algorithm: "HS256" })
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, "", fmt.Errorf("login: sign token: %w", err)
	}

	return u, tokenStr, nil
}
