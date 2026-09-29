// Package memory is an in-process repository for development and usecase
// tests. It holds data only for the lifetime of one process, so it must not be
// used by a deployment that runs more than one instance.
package memory

import (
	"github.com/m-mizutani/ariel/pkg/domain/interfaces"
)

type Memory struct {
	user            *userRepository
	slackCredential *slackCredentialRepository
	session         *sessionRepository
	slackEvent      *slackEventRepository
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{
		user:            newUserRepository(),
		slackCredential: newSlackCredentialRepository(),
		session:         newSessionRepository(),
		slackEvent:      newSlackEventRepository(),
	}
}

func (m *Memory) User() interfaces.UserRepository                       { return m.user }
func (m *Memory) SlackCredential() interfaces.SlackCredentialRepository { return m.slackCredential }
func (m *Memory) Session() interfaces.SessionRepository                 { return m.session }
func (m *Memory) SlackEvent() interfaces.SlackEventRepository           { return m.slackEvent }

func (m *Memory) Close() error {
	return nil
}
