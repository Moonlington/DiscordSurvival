package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

// MessageHandler interface handles sending of messages to players
type MessageHandler interface {
	SendMessage(p *Player, m *Message) error
	EditMessage(p *Player, m *Message) error
	GetMessage(p *Player) <-chan string
}

// Message struct is a message ingame
type Message struct {
	ID         string
	Content    string
	Choices    string
	ChatLog    []string
	Log        string
	FoodLog    string
	TeamStatus string
	Inventory  string
}

// Game struct handles the game
type Game struct {
	MsgHdlr          MessageHandler
	AllPlayers       []*Player
	Players          []*Player
	newplayers       []*Player
	MaxPlayers       int
	Inventory        map[string]int
	IsPlaying        bool
	RadioRepair      int
	Day              int
	WeatherCountdown int
	Weather          string
	Chans            map[string]<-chan string
	Rests            map[string]chan<- string
}

// AddItem handles the adding of items, amount its missing.
func (g *Game) AddItem(item string, amount int) int {
	g.Inventory[item] += amount
	if g.Inventory[item] <= 0 {
		missing := g.Inventory[item]
		g.Inventory[item] = 0
		return missing
	}
	return 0
}

// NewGame handles the creation of games
func NewGame(msghdlr MessageHandler, players []*Player) *Game {
	return &Game{msghdlr, players, players, players, len(players), map[string]int{"food": 15}, true, 0, 0, 0, "", make(map[string]<-chan string), make(map[string]chan<- string)}
}

// GetOptions asks the player what to do
func (g *Game) GetOptions(player *Player) (<-chan string, chan<- string) {
	done := make(chan string)
	rest := make(chan string)
	go func() {
		mess := new(Message)
		mess.Content = "Please note - Choosing an option cuts you off from the ingame chat."
		mess.Choices = "food | Gather for food <+ food>\nrepair | Try to repair the radio\nrest | Rest <+ health>\nchat <message> | Says something to the others.\nsuicide | Commit suicide, pls dont tho. <+ instant death>"

		err := g.MsgHdlr.SendMessage(player, mess)
		if err != nil {
			fmt.Println("fuck", err)
			return
		}

		var msgchan <-chan string

		msgchan = g.MsgHdlr.GetMessage(player)

		d := true
		timeout := time.After(time.Second * 120)

		for d {
			select {
			case returned := <-msgchan:
				if returned != "" {
					switch strings.Fields(returned)[0] {
					case "food":
						close(rest)
						delete(g.Rests, player.ID)
						d = false
						g.MsgHdlr.SendMessage(player, &Message{Content: "You decided to gather food."})
						player.NextAction = 1
						done <- "[" + player.Name + "](is going to gather food.) <+ food>"
						close(done)
					case "repair":
						close(rest)
						delete(g.Rests, player.ID)
						d = false
						g.MsgHdlr.SendMessage(player, &Message{Content: "You decided to repair the radio"})
						player.NextAction = 2
						done <- "[" + player.Name + "](is going to repair the radio.) <+ repair>"
						close(done)
					case "rest":
						close(rest)
						delete(g.Rests, player.ID)
						d = false
						g.MsgHdlr.SendMessage(player, &Message{Content: "You decided to rest"})
						player.NextAction = 3
						done <- "[" + player.Name + "](is going to rest.) <+ health>"
						close(done)
					case "chat":
						argstr := returned[5:]
						done <- "CHAT: [" + player.Name + `](` + argstr + `)`
						msgchan = g.MsgHdlr.GetMessage(player)
					case "suicide":
						close(rest)
						delete(g.Rests, player.ID)
						d = false
						g.MsgHdlr.SendMessage(player, &Message{Content: "You decided to kill yourself."})
						player.NextAction = -1
						done <- "[" + player.Name + "](is going to kill themselves.) <- " + player.Name + ">"
						close(done)
					}
				}
			case extra := <-rest:
				mess.ChatLog = append(mess.ChatLog, extra)
				g.MsgHdlr.EditMessage(player, mess)
			case <-timeout:
				close(rest)
				delete(g.Rests, player.ID)
				d = false
				g.MsgHdlr.SendMessage(player, &Message{Content: "You did nothing."})
				player.NextAction = 0
				done <- player.Name + " did nothing."
				close(done)
			}
		}
	}()
	return done, rest
}

func merge(cs ...<-chan string) <-chan string {
	var wg sync.WaitGroup
	out := make(chan string)

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c <-chan string) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// StartGame handles the starting of a Game
func (g *Game) StartGame() {
	for _, p := range g.Players {
		mess := &Message{Content: "You and others are stranded in a tundra, try to survive!\n\nHow to play\nTo input commands, just dm the word thats before the | to me."}
		g.MsgHdlr.SendMessage(p, mess)
	}
	go g.GameLoop()
}

// GameLoop handles the game itself
func (g *Game) GameLoop() {
	for g.IsPlaying {
		var AmountDone int
		for _, p := range g.Players {
			log, rest := g.GetOptions(p)
			g.Chans[p.ID] = log
			g.Rests[p.ID] = rest
		}
		d := true
		var chans []<-chan string
		for _, v := range g.Chans {
			chans = append(chans, v)
		}
		m := merge(chans...)
		for d {
			select {
			case returned := <-m:
				if !strings.HasPrefix(returned, "CHAT: ") {
					AmountDone++
				}
				for _, r := range g.Rests {
					r <- returned
				}
				if AmountDone == g.MaxPlayers {
					d = false
				}
			default:
				time.Sleep(time.Millisecond)
			}
		}
		g.PassDay()
		g.Day++
		time.Sleep(time.Second * 2)
	}
}

// PassDay handles the next day
func (g *Game) PassDay() {
	var logs []string
	chance := rand.Intn(100)
	if chance <= 10 {
		c := rand.Intn(1)
		switch c {
		case 0:
			amount := rand.Intn(3) + 3
			logs = append(logs, fmt.Sprintf("You had to move quickly, leaving behind supplies and getting injured in the process. <-%d Food>", amount))
			g.AddItem("food", -amount)
			for _, p := range g.Players {
				damage := rand.Intn(10) + 10
				p.AddHealth(-damage)
				logs = append(logs, fmt.Sprintf("%s <-%d health>", p.Name, damage))
			}
		case 1:
			amount := rand.Intn(5) + 2
			g.AddItem("food", -amount)
			logs = append(logs, fmt.Sprintf("Those darn wolves got to your food supply and ate some of it! <-%d Food>", amount))
		}
	}

	for _, p := range g.Players {
		chance := rand.Intn(100)
		switch p.NextAction {
		case -1:
			logs = append(logs, fmt.Sprintf("%s killed themselves.", p.Name))
			p.Dead = true
		case 0:
			logs = append(logs, fmt.Sprintf("%s did nothing.", p.Name))
		case 1:
			if g.Weather != "Blizzard" {
				if chance <= 10 {
					damage := rand.Intn(20) + 30
					ded := p.AddHealth(-damage)
					if ded {
						logs = append(logs, fmt.Sprintf("%s got killed by some wolves while they were gathering. <- %s>", p.Name, p.Name))
					} else {
						logs = append(logs, fmt.Sprintf("%s got bitten by some wolves while they were gathering, they came back with no food <-%d Health>", p.Name, damage))
					}
				} else if chance <= 10+20 {
					logs = append(logs, fmt.Sprintf("%s dropped the food while coming back.", p.Name))
				} else {
					amount := rand.Intn(5) + 2
					g.AddItem("food", amount)
					logs = append(logs, fmt.Sprintf("%s gathered some food. <+%d food>", p.Name, amount))
				}
			} else {
				logs = append(logs, fmt.Sprintf("%s was blinded by the blizzard and could not find any food.", p.Name))
			}
		case 2:
			if g.Weather != "Rain" {
				if chance <= 10 {
					amount := rand.Intn(5) + 1
					g.RadioRepair -= amount
					damage := rand.Intn(10) + 10
					ded := p.AddHealth(-damage)
					if ded {
						logs = append(logs, fmt.Sprintf("%s tried to repair the radio, but actually died. <-%d Repaired> <- %s>", p.Name, amount, p.Name))
					} else {
						logs = append(logs, fmt.Sprintf("%s tried to repair the radio, but failed horribly and got hurt. <-%d Repaired> <-%d Health>", p.Name, amount, damage))
					}
				} else if chance <= 10+25 {
					amount := rand.Intn(5) + 1
					g.RadioRepair -= amount
					logs = append(logs, fmt.Sprintf("%s tried to repair the radio, but failed horribly. <-%d Repaired>", p.Name, amount))
				} else {
					amount := rand.Intn(5) + 1
					g.RadioRepair += amount
					logs = append(logs, fmt.Sprintf("%s repaired the radio a little. <+%d Repaired>", p.Name, amount))
				}
			} else {
				damage := rand.Intn(10) + 20
				ded := p.AddHealth(-damage)
				if ded {
					logs = append(logs, fmt.Sprintf("%s tried to repair the radio, but got electrocuted and died. <- %s>", p.Name, p.Name))
				} else {
					logs = append(logs, fmt.Sprintf("%s tried to repair the radio, but got electrocuted. <-%d Health>", p.Name, damage))
				}
			}
		case 3:
			if chance <= 20 {
				logs = append(logs, fmt.Sprintf("%s had nightmares while sleeping, he didn't sleep well...", p.Name))
			} else {
				amount := rand.Intn(15) + 10
				p.AddHealth(amount)
				logs = append(logs, fmt.Sprintf("%s slept and healed up a bit. <+%d Health>", p.Name, amount))
			}
		default:
			logs = append(logs, fmt.Sprintf("%s did something weird, i dunno.", p.Name))
		}
	}

	message := new(Message)

	foodlog := g.EatFood()

	message.FoodLog = foodlog

	g.newplayers = g.Players[:0]
	for _, x := range g.Players {
		if !x.Dead {
			g.newplayers = append(g.newplayers, x)
		}
	}

	message.Log = strings.Join(logs, "\n")

	if g.WeatherCountdown <= 0 {
		c := rand.Intn(2)
		switch c {
		case 0:
			g.Weather = "Rain"
			g.WeatherCountdown = rand.Intn(1) + 1
		case 1:
			g.Weather = "Snow"
			g.WeatherCountdown = rand.Intn(1) + 1
		case 2:
			g.Weather = "Blizzard"
			g.WeatherCountdown = rand.Intn(1) + 1
		}
	} else {
		g.WeatherCountdown--
	}

	message.Content = "Current weather: " + g.Weather

	var tstatus string
	for _, p := range g.newplayers {
		tstatus += "\n" + p.String()
	}

	message.TeamStatus = tstatus

	var inv string
	for k, v := range g.Inventory {
		inv += fmt.Sprintf("\n<%s %d>", k, v)
	}

	message.Inventory = inv

	for _, p := range g.Players {
		g.MsgHdlr.SendMessage(p, message)
		p.NextAction = 0
	}
	g.Players = g.newplayers
	if g.RadioRepair >= 100 {
		g.Win()
	}
	if len(g.Players) <= 0 {
		g.Lose()
	}
}

// Win handles winning
func (g *Game) Win() {
	mess := &Message{Content: "You finally repair the radio, you hear a voice coming out of it. It's asking who is on the other side and you answer. A short while later a ship comes down to pick you up\nNicely done."}
	for _, p := range g.AllPlayers {
		g.MsgHdlr.SendMessage(p, mess)
	}
	g.IsPlaying = false
	newingame := ingame[:0]
	found := false
	for _, i := range ingame {
		for _, p := range g.AllPlayers {
			if i == p.ID {
				found = true
			}
		}
		if !found {
			newingame = append(newingame, i)
		}
	}
	ingame = newingame
}

// Lose handles losing
func (g *Game) Lose() {
	mess := &Message{Content: "Yall died, rip."}
	for _, p := range g.AllPlayers {
		g.MsgHdlr.SendMessage(p, mess)
	}
	g.IsPlaying = false
	newingame := ingame[:0]
	found := false
	for _, i := range ingame {
		for _, p := range g.AllPlayers {
			if i == p.ID {
				found = true
			}
		}
		if !found {
			newingame = append(newingame, i)
		}
	}
	ingame = newingame
}

// EatFood handles eating of food
func (g *Game) EatFood() string {
	var foodlog string
	sort.Slice(g.newplayers, func(i, j int) bool { return g.newplayers[i].Hunger > g.newplayers[j].Hunger })
	for _, p := range g.newplayers {
		foodlog += "\n"
		if !p.Dead {
			if g.Inventory["food"] > 0 {
				g.AddItem("food", -1)
				p.AddHunger(-15)
				foodlog += p.Name + " ate some food. <-15 hunger>"
			} else {
				state := p.AddHunger(20)
				switch state {
				case 0:
					foodlog += p.Name + " got hungry. <+20 hunger>"
				case 1:
					foodlog += p.Name + " is starving! <max hunger>"
				case 2:
					foodlog += p.Name + " starved to death."
					p.Dead = true
				}
			}
		}
	}
	return foodlog
}
