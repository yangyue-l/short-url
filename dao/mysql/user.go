package mysql

import "short-url/models"

func CreateUser(user *models.User) error {
	return db.Create(user).Error
}

func GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByID(id int64) (*models.User, error) {
	var user models.User
	err := db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func DeleteUser(id int64) error {
	return db.Delete(&models.User{}, id).Error
}
