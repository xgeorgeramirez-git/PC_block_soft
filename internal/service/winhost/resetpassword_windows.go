//go:build windows

package winhost

import (
	"errors"

	"pcblock/internal/ipc"
	"pcblock/internal/launch"
)

// ResetPasswordNoWindow сбрасывает пароль родителя без окна (--reset-password --new-password=…,
// сценарий S25 в CI). Работает только при включённом тестовом режиме. Права проверяет служба (R20):
// вызов от обычного пользователя получит отказ «Нужны права администратора». Итог пишется в --out.
func ResetPasswordNoWindow(opts launch.Options) error {
	info := map[string]any{}
	return finish(opts["out"], info, resetPasswordNoWindow(opts))
}

func resetPasswordNoWindow(opts launch.Options) error {
	if !testModeOn() {
		return errTestModeOff
	}
	if opts["new-password"] == "" {
		return errors.New("Не указан --new-password")
	}
	c, err := dial()
	if err != nil {
		return err
	}
	defer c.Close()
	req := ipc.ResetPasswordRequest{NewPassword: opts["new-password"]}
	if err := ipc.Call(c, ipc.CmdResetPassword, "", req, nil); err != nil {
		return replyText(err)
	}
	return nil
}
