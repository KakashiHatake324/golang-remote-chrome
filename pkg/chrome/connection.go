package chrome

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KakashiHatake324/mockjs"
	"github.com/gorilla/websocket"
)

type Command struct {
	Method string         `json:"method"`
	Id     int            `json:"id"`
	Params map[string]any `json:"params"`
}

type CommandResponse struct {
	Type        string `json:"type"`
	Value       string `json:"value"`
	Subtype     string `json:"subtype"`
	ClassName   string `json:"className"`
	Description string `json:"description"`
	ObjectId    string `json:"objectId"`
}

func (p *Page) NewCommand(method string, params map[string]any) *Command {
	p.messageCounter++
	return &Command{Method: method, Params: params, Id: p.messageCounter}
}

func (c *Command) string() string {
	json, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(json)
}

func (p *Page) send(command *Command) error {
	if p.verbose {
		p.logger.Info(fmt.Sprintf("sending command: %s", command.string()))
	}
	return p.wsConn.WriteMessage(websocket.TextMessage, []byte(command.string()))
}

func (p *Page) sendAndReceive(command *Command) (*CommandResponse, error) {
	err := p.send(command)
	if err != nil {
		return nil, err
	}

	if p.verbose {
		p.logger.Info(fmt.Sprintf("waiting for response from command: %s", command.string()))
	}
	for {
		select {
		case message := <-p.communicator:
			if p.verbose {
				p.logger.Info(fmt.Sprintf("received message id: %d", mockjs.Math.ToInt(message.(map[string]any)["id"])))
			}
			if mockjs.Math.ToInt(message.(map[string]any)["id"]) < command.Id {
				if p.verbose {
					p.logger.Info(fmt.Sprintf("received message: %s", mockjs.InitWindow().JSON.Stringify(message)))
				}
			}
			if mockjs.Math.ToInt(message.(map[string]any)["id"]) == command.Id {
				if p.verbose {
					p.logger.Info(fmt.Sprintf("received message: %s", mockjs.InitWindow().JSON.Stringify(message)))
				}
				response, err := parseResponse(mockjs.InitWindow().JSON.Stringify(message))
				if err != nil {
					return nil, err
				}

				if _, ok := response["error"]; ok {
					return nil, fmt.Errorf("error: %s", response["error"].(map[string]any)["message"])
				}

				switch result := response["result"].(type) {
				case string:
					return &CommandResponse{Type: "string", Value: result}, nil
				case map[string]any:
					if _, ok := result["frame"]; ok {
						p.frameId = result["frame"].(string)
					}
					if _, ok := result["result"]; ok {
						var value CommandResponse
						json.Unmarshal([]byte(mockjs.InitWindow().JSON.Stringify(result["result"])), &value)
						if strings.HasPrefix(value.Description, "TypeError") {
							return nil, fmt.Errorf("error: %s", value.Description)
						}
						return &value, nil
					}
					return &CommandResponse{Type: "string", Value: mockjs.InitWindow().JSON.Stringify(result)}, nil
				case []any:
					return &CommandResponse{Type: "string", Value: mockjs.InitWindow().JSON.Stringify(result)}, nil
				}
			}
		}
	}
}

// receive receives a message from the websocket
func (p *Page) receive() (string, error) {
	messageType, message, err := p.wsConn.ReadMessage()
	if err != nil {
		return "", err
	}

	if messageType != websocket.TextMessage {
		return "", fmt.Errorf("expected text message, got %d", messageType)
	}

	return string(message), nil
}

func parseResponse(response string) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		return nil, err
	}
	return data, nil
}
