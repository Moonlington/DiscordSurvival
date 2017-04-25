package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
)

const maxplayers = 4
const maxdays = 50

var ingame []string
var nextplayers []*Player
var loadinggame *Game

var conf *Config

// Config struct handles the Token and Prefix of DiscordSurvival
type Config struct {
	Token  string
	Prefix string
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

// AddNewPlayer handles the adding of a new player to a game
func AddNewPlayer(s *discordgo.Session, u *discordgo.User) {
	for _, i := range ingame {
		if i == u.ID {
			return
		}
	}
	dm, _ := s.UserChannelCreate(u.ID)
	np := NewPlayer(u.ID, u.Username)
	ingame = append(ingame, u.ID)
	nextplayers = append(nextplayers, np)
	s.ChannelMessageSend(dm.ID, fmt.Sprintf("New player: %s [%d/%d]", u.String(), len(nextplayers), maxplayers))
	for _, p := range nextplayers {
		if p.ID != u.ID {
			pdm, _ := s.UserChannelCreate(p.ID)
			s.ChannelMessageSend(pdm.ID, fmt.Sprintf("New player: %s [%d/%d]", u.String(), len(nextplayers), maxplayers))
		}
	}
	fmt.Println(fmt.Sprintf("New player: %s [%d/%d]", u.String(), len(nextplayers), maxplayers))
	if len(nextplayers) >= maxplayers {
		dhandler := new(DiscordHandler)
		dhandler.Sess = s
		loadinggame = NewGame(dhandler, nextplayers)
		dhandler.Game = loadinggame
		fmt.Println("Game started")
		loadinggame.StartGame()
		loadinggame = nil
		nextplayers = []*Player{}
	}
}

// AboutCommand handles the command "About"
func AboutCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	em := &discordgo.MessageEmbed{}
	em.Color = 16411649
	me, _ := s.User("@me")
	em.Author = &discordgo.MessageEmbedAuthor{
		Name:    "DiscordSurvival",
		IconURL: discordgo.EndpointUserAvatar(me.ID, me.Avatar),
	}
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
		Name:   "Creator",
		Value:  "<@139386544275324928>",
		Inline: true,
	})
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
		Name:   "About",
		Value:  "```I am DiscordSurvival, a multiplayer survival game bot made by Floretta. I am still in development and very unbalanced. I hope you like my game though!```",
		Inline: true,
	})
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
		Name:   "Development Server",
		Value:  "https://discord.gg/pPxa93F",
		Inline: true,
	})
	s.ChannelMessageSendEmbed(m.ChannelID, em)
}

// InviteCommand handles the command "Invite"
func InviteCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	em := &discordgo.MessageEmbed{}
	em.Color = 16411649
	me, _ := s.User("@me")
	em.Author = &discordgo.MessageEmbedAuthor{
		Name:    "DiscordSurvival",
		IconURL: discordgo.EndpointUserAvatar(me.ID, me.Avatar),
	}
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
		Name:   "Invite URL",
		Value:  "https://discordapp.com/oauth2/authorize?client_id=" + me.ID + "&scope=bot",
		Inline: true,
	})
	s.ChannelMessageSendEmbed(m.ChannelID, em)
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	if strings.HasPrefix(strings.ToLower(m.Content), conf.Prefix+"joingame") && loadinggame == nil {
		AddNewPlayer(s, m.Author)
	} else if strings.HasPrefix(strings.ToLower(m.Content), conf.Prefix+"about") {
		AboutCommand(s, m)
	} else if strings.HasPrefix(strings.ToLower(m.Content), conf.Prefix+"invite") {
		InviteCommand(s, m)
	}
}
