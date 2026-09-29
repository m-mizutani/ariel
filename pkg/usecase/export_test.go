package usecase

import "time"

func (uc *AuthUseCase) SetNowForTest(now func() time.Time) { uc.now = now }

func (uc *SlackEventUseCase) SetNowForTest(now func() time.Time) { uc.now = now }

var TokenAADForTest = tokenAAD
