package logic

import (
	"errors"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"
	"short-url/pkg/jwt"
	"short-url/pkg/snowflake"
	"short-url/settings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserLogin = errors.New("用户名或密码错误")
)

// userRole 根据用户名判断角色（配置文件中定义管理员列表）
func userRole(username string) string {
	if settings.Cfg.IsAdmin(username) {
		return "admin"
	}
	return "user"
}

func UserRegister(p *models.ParamRegisterRequest) (*models.ParamRegisterResponse, error) {
	userID := snowflake.GenID()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:        userID,
		Username:  p.Username,
		Password:  string(hashedPassword),
		Email:     p.Email,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := mysql.CreateUser(user); err != nil {
		return nil, err
	}

	resp := &models.ParamRegisterResponse{
		UserID:   userID,
		Username: p.Username,
	}

	return resp, nil
}

func UserLogin(p *models.ParamLoginRequest) (*models.ParamLoginResponse, error) {
	user, err := mysql.GetUserByUsername(p.Username)
	if err != nil {
		return nil, ErrUserLogin
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(p.Password)); err != nil {
		return nil, ErrUserLogin
	}
	role := userRole(user.Username)
	aToken, rToken, err := jwt.GenToken(user.ID, user.Username, role)
	if err != nil {
		return nil, err
	}
	resp := &models.ParamLoginResponse{
		Token:        aToken,
		RefreshToken: rToken,
		ExpireAt:     time.Now().Add(jwt.AccessTokenExpireDuration).Format(time.RFC3339),
	}
	return resp, nil
}

func UserRefresh(tokenString string) (*models.ParamRefreshResponse, error) {
	mc, err := jwt.ParseTokenForRefresh(tokenString)
	if err != nil {
		return nil, err
	}
	role := userRole(mc.Username)
	aToken, rToken, err := jwt.GenToken(mc.UserID, mc.Username, role)
	if err != nil {
		return nil, err
	}
	return &models.ParamRefreshResponse{
		Token:        aToken,
		RefreshToken: rToken,
		ExpireAt:     time.Now().Add(jwt.AccessTokenExpireDuration).Format(time.RFC3339),
	}, nil
}

func DeleteUser(userID int64) error {
	// 查出用户所有短码，事务提交后清理 Redis 缓存
	codes, err := mysql.GetShortCodesByUserID(userID)
	if err != nil {
		return err
	}

	if err := mysql.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.URL{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", userID).Delete(&models.User{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	// 异步清理 Redis 缓存
	if len(codes) > 0 {
		client := redis.GetClient()
		go func() {
			for _, code := range codes {
				redis.DeleteCacheByClient(client, code)
			}
		}()
	}

	return nil
}
