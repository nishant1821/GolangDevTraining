package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nishantks908/urlshortener/internal/handler"
	"github.com/nishantks908/urlshortener/internal/service"
	"github.com/nishantks908/urlshortener/internal/store"
)

func main() {
	// 1. Storeroom — sabse andar wali layer, isliye pehle
	st := store.NewInMemoryStore()

	// 2. Chef — store ko andar daala (injection)
	svc := service.New(st)

	// 3. Waiter — service ko andar daala
	h := handler.New(svc)

	// 4. Router — kaunsa URL kis waiter-method pe jaaye
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Post("/shorten", h.Shorten) // lambi URL → chhota code
	})
	r.Get("/{code}", h.Redirect) // chhota code → 302 redirect

	log.Println("shutter khul gaya → :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
