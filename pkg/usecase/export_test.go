package usecase

import "time"

func (uc *AuthUseCase) SetNowForTest(now func() time.Time) { uc.now = now }

func (uc *SlackEventUseCase) SetNowForTest(now func() time.Time) { uc.now = now }

var TokenAADForTest = tokenAAD

func (uc *GoogleWorkspaceUseCase) SetNowForTest(now func() time.Time) { uc.now = now }

var GoogleTokenAADForTest = googleTokenAAD

func (a *NotionAccess) SetNowForTest(now func() time.Time) { a.now = now }

var NotionTokenAADForTest = notionTokenAAD
