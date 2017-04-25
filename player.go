package main

import (
	"fmt"
)

// Player struct handles the player
type Player struct {
	ID             string
	Name           string
	Health         int
	Hunger         int
	CriticalHunger bool
	NextAction     int
	Dead           bool
}

// NewPlayer handles the creation of players
func NewPlayer(ID, name string) *Player {
	return &Player{ID, name, 100, 0, false, 0, false}
}

func (p *Player) String() string {
	return fmt.Sprintf("[%-12s](HP: %3d)<Hunger: %3d>", p.Name, p.Health, p.Hunger)
}

// AddHealth handles the adding of health, returns true if dead
func (p *Player) AddHealth(amount int) bool {
	p.Health += amount
	if p.Health > 100 {
		p.Health = 100
	}
	if p.Health <= 0 {
		p.Health = 0
		p.Dead = true
		return true
	}
	return false
}

// AddHunger handles the adding of hunger, returns 0 normally, 1 if critical state, 2 if dead
func (p *Player) AddHunger(amount int) int {
	p.Hunger += amount
	if p.Hunger >= 100 {
		p.Hunger = 100
		if p.CriticalHunger {
			return 2
		}
		p.CriticalHunger = true
		return 1
	}
	if p.Hunger <= 0 {
		p.Hunger = 0
	}
	return 0
}
