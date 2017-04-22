package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
)

var conf *Config

// Config struct handles the Token and Prefix of DiscordSurvival
type Config struct {
	Token  string
	Prefix string
}

// Player struct handles the player
type Player struct {
	User           *discordgo.User
	DM             *discordgo.Channel
	Name           string
	Health         int
	Hunger         int
	CriticalHunger bool
	NextAction     int
	Dead           bool
}

func (p *Player) String() string {
	return fmt.Sprintf("[%-10s](HP: %3d)<Hunger: %3d>", p.Name, p.Health, p.Hunger)
}

// Dm handles dmming to a person
func (p *Player) Dm(s *discordgo.Session, send string) (*discordgo.Message, error) {
	return s.ChannelMessageSend(p.DM.ID, send)
}

// DmEmbed handles dmming embeds to a person
func (p *Player) DmEmbed(s *discordgo.Session, send *discordgo.MessageEmbed) (*discordgo.Message, error) {
	return s.ChannelMessageSendEmbed(p.DM.ID, send)
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

// Game struct handles the game
type Game struct {
	Sess             *discordgo.Session
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

// NewPlayer handles the creation of players
func NewPlayer(user *discordgo.User, dm *discordgo.Channel) *Player {
	return &Player{user, dm, user.Username, 100, 0, false, 0, false}
}

// NewGame handles the creation of games
func NewGame(s *discordgo.Session, players []*Player) *Game {
	return &Game{s, players, players, players, len(players), map[string]int{"food": 15}, true, 0, 0, 0, "", make(map[string]<-chan string), make(map[string]chan<- string)}
}

// MakeEmbedMessage creates an embed the bot can send
func (g *Game) MakeEmbedMessage(title, description string) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{}
	em.Color = 16411649
	em.Author = &discordgo.MessageEmbedAuthor{Name: title}
	em.Description = description
	return em
}

// GetOptions asks the player what to do
func (g *Game) GetOptions(player *Player) (<-chan string, chan<- string) {
	done := make(chan string)
	rest := make(chan string)
	go func() {
		em := g.MakeEmbedMessage(fmt.Sprintf("Discord Survival | Day %d | Radio: %d%%", g.Day, g.RadioRepair), "```md\nfood | Gather for food <+ food>\nrepair | Try to repair the radio\nrest | Rest <+ health>\nsuicide | Commit suicide, pls dont tho. <+ instant death>```")

		msg, err := player.DmEmbed(g.Sess, em)
		if err != nil {
			fmt.Println("fuck")
			return
		}

		msgchan := addMessageQueue(g.Sess, func(m *discordgo.MessageCreate) bool {
			if m.ChannelID == player.DM.ID && m.Author.ID == player.User.ID {
				switch strings.ToLower(m.Content) {
				case "food":
					return true
				case "repair":
					return true
				case "rest":
					return true
				case "suicide":
					return true
				default:
					return false
				}
			}
			return false
		})

		d := true
		timeout := time.After(time.Second * 30)

		for d {
			select {
			case returned := <-msgchan:
				if returned != nil {
					switch strings.ToLower(returned.Content) {
					case "food":
						close(rest)
						delete(g.Rests, player.User.ID)
						d = false
						player.Dm(g.Sess, "You decided to gather food.")
						player.NextAction = 1
						done <- player.Name + " is going to gather food. <+ food>"
						close(done)
					case "repair":
						close(rest)
						delete(g.Rests, player.User.ID)
						d = false
						player.Dm(g.Sess, "You decided to repair the radio")
						player.NextAction = 2
						done <- player.Name + " is going to repair the radio. <+ repair>"
						close(done)
					case "rest":
						close(rest)
						delete(g.Rests, player.User.ID)
						d = false
						player.Dm(g.Sess, "You decided to rest")
						player.NextAction = 3
						done <- player.Name + " is going to rest. <+ health>"
						close(done)
					case "suicide":
						close(rest)
						delete(g.Rests, player.User.ID)
						d = false
						player.Dm(g.Sess, "You decided to kill yourself.")
						player.NextAction = -1
						done <- player.Name + " is going to kill themselves. <- " + player.Name + ">"
						close(done)
					}
				}
			case extra := <-rest:
				em.Description += "```md\n" + extra + "```"
				g.Sess.ChannelMessageEditEmbed(player.DM.ID, msg.ID, em)
			case <-timeout:
				close(rest)
				delete(g.Rests, player.User.ID)
				d = false
				player.Dm(g.Sess, "You did nothing.")
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
	em := g.MakeEmbedMessage("Discord Survival | Introduction", fmt.Sprintf("You and others are stranded in a tundra, try to survive!"))
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: "How to play", Value: "To input commands, just dm the word thats before the `|` to me."})
	for _, p := range g.Players {
		p.DmEmbed(g.Sess, em)
	}
	go g.GameLoop()
}

// GameLoop handles the game itself
func (g *Game) GameLoop() {
	for g.IsPlaying {
		var logs []string
		for _, p := range g.Players {
			log, rest := g.GetOptions(p)
			g.Chans[p.User.ID] = log
			g.Rests[p.User.ID] = rest
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
				logs = append(logs, returned)
				for _, r := range g.Rests {
					r <- returned
				}
				if len(logs) == g.MaxPlayers {
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
	if chance <= 5 {
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

	foodlog := g.EatFood()

	g.newplayers = g.Players[:0]
	for _, x := range g.Players {
		if !x.Dead {
			g.newplayers = append(g.newplayers, x)
		}
	}

	desc := "```md\n# Logs\n" + strings.Join(logs, "\n")

	if g.WeatherCountdown <= 0 {
		c := rand.Intn(2)
		switch c {
		case 0:
			g.Weather = "Rain"
			g.WeatherCountdown = rand.Intn(2) + 1
		case 1:
			g.Weather = "Snow"
			g.WeatherCountdown = rand.Intn(2) + 1
		case 2:
			g.Weather = "Blizzard"
			g.WeatherCountdown = rand.Intn(2) + 1
		}
	} else {
		g.WeatherCountdown--
	}

	desc += fmt.Sprintf("\n# The current weather is: %s", g.Weather) + "``` ```md\n" + foodlog + "``` ```md\n# Team stats"
	for _, p := range g.newplayers {
		desc += "\n" + p.String()
	}

	desc += "``` ```md\n# Inventory"
	for k, v := range g.Inventory {
		desc += fmt.Sprintf("\n<%s %d>", k, v)
	}
	desc += "```"

	em := g.MakeEmbedMessage(fmt.Sprintf("Discord Survival Results | Day %d | Radio: %d%%", g.Day, g.RadioRepair), desc)

	for _, p := range g.Players {
		p.DmEmbed(g.Sess, em)
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
	em := g.MakeEmbedMessage("Discord Survival | You win!", "You finally repair the radio, you hear a voice coming out of it. It's asking who is on the other side and you answer. A short while later a ship comes down to pick you up\nNicely done.")
	for _, p := range g.AllPlayers {
		p.DmEmbed(g.Sess, em)
	}
	g.IsPlaying = false
	newingame := ingame[:0]
	found := false
	for _, i := range ingame {
		for _, p := range g.AllPlayers {
			if i == p.User.ID {
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
	em := g.MakeEmbedMessage("Discord Survival | You lose.", "Yall died, rip.")
	for _, p := range g.AllPlayers {
		p.DmEmbed(g.Sess, em)
	}
	g.IsPlaying = false
	newingame := ingame[:0]
	found := false
	for _, i := range ingame {
		for _, p := range g.AllPlayers {
			if i == p.User.ID {
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
	sort.Slice(g.newplayers, func(i, j int) bool { return g.newplayers[i].Hunger > g.newplayers[j].Hunger })

	foodlog := "# Feeding log"

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

func main() {

	_, err := toml.DecodeFile("config.toml", &conf)
	if os.IsNotExist(err) {
		fmt.Println("No config file found.")
	}

	rand.Seed(time.Now().Unix())
	dg, err := discordgo.New("Bot " + conf.Token)
	if err != nil {
		fmt.Println(err)
		return
	}

	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

const maxplayers = 4

var ingame []string
var nextplayers []*Player
var loadinggame *Game

func addMessageQueue(s *discordgo.Session, f func(m *discordgo.MessageCreate) bool) <-chan *discordgo.MessageCreate {
	c := make(chan *discordgo.MessageCreate)
	go func() {
		var deletdis func()
		x := make(chan *discordgo.MessageCreate)
		deletdis = s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
			if f(m) {
				x <- m
			}
		})
		c <- <-x
		close(x)
		close(c)
		deletdis()
	}()
	return c
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	if strings.HasPrefix(strings.ToLower(m.Content), conf.Prefix+"joingame") && loadinggame == nil {
		for _, i := range ingame {
			if i == m.Author.ID {
				return
			}
		}
		dm, _ := s.UserChannelCreate(m.Author.ID)
		np := NewPlayer(m.Author, dm)
		ingame = append(ingame, m.Author.ID)
		nextplayers = append(nextplayers, np)
		for _, p := range nextplayers {
			p.Dm(s, fmt.Sprintf("New player: %s [%d/%d]", m.Author.String(), len(nextplayers), maxplayers))
		}
		fmt.Println(fmt.Sprintf("New player: %s [%d/%d]", m.Author.String(), len(nextplayers), maxplayers))
		if len(nextplayers) >= maxplayers {
			loadinggame = NewGame(s, nextplayers)
			fmt.Println("Game started")
			loadinggame.StartGame()
			loadinggame = nil
			nextplayers = []*Player{}
		}
	}
}
