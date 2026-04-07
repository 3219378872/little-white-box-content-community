package validator

import (
	"errx"
	"regexp"
	"unicode"
	"unicode/utf8"
)

var phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)

func ValidatePhone(phone string) error {
	if !phoneRegex.MatchString(phone) {
		return errx.New(errx.ParamError, "非法的手机号")
	}
	return nil
}

func CheckPasswordStrength(password string) (bool, error) {
	if len(password) < 8 || len(password) > 64 {
		return false, errx.New(errx.ParamError, "密码过长或过短")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		}
	}
	isStrength := hasUpper && hasLower && hasDigit
	if isStrength {
		return isStrength, nil
	}

	return false, errx.New(errx.ParamError, "密码强度过弱，至少需要包含大小写字母和数字")
}

func ValidateUserName(userName string) error {
	n := utf8.RuneCountInString(userName)
	if n <= 50 && n >= 6 {
		return nil
	}
	return errx.New(errx.ParamError, "用户名长度应在6~50之间")
}

//// 需修改types包下的自动生成结构体，较难维护，此处仅存放示例代码
//var validate *validator.Validate
//
//func init() {
//	validate = validator.New()
//	_ = validate.RegisterValidation("phone", validatePhoneByValidator)
//}
//
//func validatePhoneByValidator(fl validator.FieldLevel) bool {
//	return phoneRegex.MatchString(fl.Field().String())
//}
