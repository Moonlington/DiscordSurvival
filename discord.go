package main

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// DiscordHandler is a MessageHandler for Discord
type DiscordHandler struct {
	Sess *discordgo.Session
	Game *Game
}

// MakeEmbedMessage creates an embed the bot can send
func (d *DiscordHandler) MakeEmbedMessage(m *Message) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{}
	em.Color = 16411649
	em.Author = &discordgo.MessageEmbedAuthor{Name: fmt.Sprintf("Discord Survival | Day %d | Radio: %d%%", d.Game.Day, d.Game.RadioRepair)}

	if m.Content != "" {
		em.Description = "```md\n" + m.Content + "\n```"
	}
	if m.Choices != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Choices",
			Value: "```md\n" + m.Choices + "```",
		})
	}
	if len(m.ChatLog) != 0 {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Chat Log",
			Value: "```md\n" + strings.Join(m.ChatLog, "\n") + "```",
		})
	}
	if m.Log != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Log",
			Value: "```md\n" + m.Log + "```",
		})
	}
	if m.FoodLog != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Food Log",
			Value: "```md\n" + m.FoodLog + "```",
		})
	}
	if m.TeamStatus != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Team Status",
			Value: "```md\n" + m.TeamStatus + "```",
		})
	}
	if m.Inventory != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name:  "Inventory",
			Value: "```md\n" + m.Inventory + "```",
		})
	}

	return em
}

// SendMessage handles the sending of a message to a Player
func (d *DiscordHandler) SendMessage(p *Player, m *Message) error {
	dm, err := d.Sess.UserChannelCreate(p.ID)
	if err != nil {
		return err
	}

	em := d.MakeEmbedMessage(m)

	msg, err := d.Sess.ChannelMessageSendEmbed(dm.ID, em)
	if err != nil {
		return err
	}
	m.ID = msg.ID
	return nil
}

// EditMessage handles the editing of a message
func (d *DiscordHandler) EditMessage(p *Player, m *Message) error {
	dm, err := d.Sess.UserChannelCreate(p.ID)
	if err != nil {
		return err
	}

	em := d.MakeEmbedMessage(m)

	msg, err := d.Sess.ChannelMessageEditEmbed(dm.ID, m.ID, em)
	if err != nil {
		return err
	}
	m.ID = msg.ID
	return nil
}

// GetMessage handles the editing of a message
func (d *DiscordHandler) GetMessage(p *Player) <-chan string {
	c := make(chan string)
	go func() {
		dm, err := d.Sess.UserChannelCreate(p.ID)
		if err != nil {
			return
		}
		msg := <-addMessageQueue(d.Sess, func(m *discordgo.MessageCreate) bool {
			if m.ChannelID == dm.ID && m.Author.ID == p.ID {
				switch strings.Fields(m.ContentWithMentionsReplaced())[0] {
				case "food":
					return true
				case "repair":
					return true
				case "rest":
					return true
				case "chat":
					return true
				case "suicide":
					return true
				default:
					return false
				}
			}
			return false
		})
		c <- msg.ContentWithMentionsReplaced()
		close(c)
	}()
	return c
}
