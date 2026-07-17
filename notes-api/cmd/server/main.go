package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"notes-api/internal/auth"
	"notes-api/internal/note"
	"notes-api/internal/user"
)

// getEnv — Node: process.env.DB_HOST || "localhost" jaisa.
// Credentials source code mein hardcode NAHI karte (git mein commit ho jaate hain, leak ho sakte hain) —
// isliye env vars se padhte hain, saath mein local-dev ke liye harmless fallback rakhte hain.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	// godotenv.Load() — Node: require("dotenv").config() jaisa hi, ".env" file padh ke
	// os.Getenv() ke through available kar deta hai. Error ignore kar rahe hain jaanbujh kar —
	// production mein .env file hoti hi nahi, wahan real env vars already set hote hain (Docker/systemd se).
	_ = godotenv.Load()

	// Postgres connect karne ke liye DSN (connection string) banate hain —
	// Node: `postgres://user:pass@host:port/dbname` jaisa hi info, bas GORM ka format space-separated hai
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", ""),
		getEnv("DB_NAME", "notes-api"),
		getEnv("DB_PORT", "5432"),
	)

	// gorm.Open == Node ke sequelize.authenticate() / mongoose.connect() jaisa
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("db connect nahi hua:", err)
	}

	// AutoMigrate — struct dekh ke tables bana/update karta hai.
	// Node: sequelize.sync() / Prisma migrate jaisa, bas har startup pe safe-run hota hai
	// (existing data delete nahi karta, sirf missing columns/tables add karta hai)
	if err := db.AutoMigrate(&user.User{}, &note.Note{}); err != nil {
		log.Fatal("migration fail:", err)
	}

	// Layers wire karo — store pehle (sabse andar), phir handler (store ko andar leta hai)
	userStore := user.NewStore(db)
	userHandler := user.NewHandler(userStore)

	noteStore := note.NewStore(db)
	noteHandler := note.NewHandler(noteStore)

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// /auth routes — public, koi token nahi chahiye
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", userHandler.Register)
		r.Post("/login", userHandler.Login)
	})

	// YOUR TURN: /notes route group likh — ye sab PROTECTED hone chahiye.
	// Node: const notesRouter = express.Router(); notesRouter.use(authMiddleware); app.use("/notes", notesRouter)
	//
	// chi mein pattern:
	//
	//	r.Route("/notes", func(r chi.Router) {
	//	    r.Use(auth.RequireAuth)              // is group ke andar HAR route se pehle chalega
	//	    r.Get("/", noteHandler.List)
	//	    r.Post("/", noteHandler.Create)
	//	    r.Get("/{id}", noteHandler.Get)
	//	    r.Put("/{id}", noteHandler.Update)
	//	    r.Delete("/{id}", noteHandler.Delete)
	//	})
	//
	// self-check: r.Use(auth.RequireAuth) yahan is Route() block ke ANDAR hai, bahar nahi —
	// isse sirf /notes/* routes protected hote hain, /auth/* aur /health free rehte hain.
	// Agar r.Use() ko sabse bahar (top-level r pe) laga dete, toh /auth/login bhi token maangta — bug!

	r.Route("/notes", func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/", noteHandler.List)
		r.Post("/", noteHandler.Create)
		r.Get("/{id}", noteHandler.Get)
		r.Put("/{id}", noteHandler.Update)
		r.Delete("/{id}", noteHandler.Delete)

	})
	log.Println("server is running :8080 pe")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
