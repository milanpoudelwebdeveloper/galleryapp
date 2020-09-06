package models

import (
	"errors"

	"github.com/jinzhu/gorm"

	"../hash"
	"../rand"

	//This is
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"golang.org/x/crypto/bcrypt"
)

var (
	//ErrNotFound is returned when resouce can't be found in database
	ErrNotFound = errors.New("models: resourcess not found")

	//ErrInvalidID is returned when an invalid ID is provided
	//to a method like Delete.
	ErrInvalidID = errors.New("models:ID must be > 0")

	//ErrInvalidEmail is
	ErrInvalidEmail = errors.New("models:invalid email address provided")

	//ErrInvalidPassword is returned when an invalid password is used when attempting to authenticate a user.
	ErrInvalidPassword = errors.New("models: incorrect password provided")
)

const userPwPepper = "secret-random-string"
const hmacSecretKey = "secret-hmac-key"

//UserDB is used to interact with the users database.For single users, any error but ErrNotFound should probably result in 500 error.
type UserDB interface {
	//Methods for Querying for Single User
	ByID(id uint) (*User, error)
	ByEmail(email string) (*User, error)
	ByRemember(token string) (*User, error)

	//Methods for altering users
	Create(user *User) error
	Update(user *User) error
	Delete(id uint) error

	//Used to close a DB connection
	Close() error

	//Migration helpers
	AutoMigrate() error
	DestructiveReset() error
}

//NewUserService is
func NewUserService(connectionInfo string) (*UserService, error) {
	ug, err := newUserGorm(connectionInfo)
	if err != nil {
		return nil, err
	}
	return &UserService{
		UserDB: &userValidator{
			UserDB: ug,
		},
	}, nil
}

//UserService is
type UserService struct {
	UserDB
}

type userValidator struct {
	UserDB
}

func newUserGorm(connectionInfo string) (*userGorm, error) {
	db, err := gorm.Open("postgres", connectionInfo)
	if err != nil {
		return nil, err
	}

	db.LogMode(true)
	hmac := hash.NewHMAC(hmacSecretKey)
	return &userGorm{
		db:   db,
		hmac: hmac,
	}, nil
}

var _ UserDB = &userGorm{}

type userGorm struct {
	db   *gorm.DB
	hmac hash.HMAC
}

//ByID will look up by the id provided
//case 1-user,error-nil
//case 2-nil, ErrNotFound
//case 3-nil,otherError
func (ug *userGorm) ByID(id uint) (*User, error) {
	var user User
	db := ug.db.Where("id=?", id)
	err := first(db, &user)
	return &user, err
}

//ByEmail looks up a user with the given email address and
//returns that user
func (ug *userGorm) ByEmail(email string) (*User, error) {
	var user User
	db := ug.db.Where("email=?", email)
	err := first(db, &user)
	return &user, err

}

//ByRemember looks up a user with the given remember token and returns that user.This method will handle hashing the token for us.Errors are the same as ByEmail and BYID
func (ug *userGorm) ByRemember(token string) (*User, error) {
	var user User
	rememberHash := ug.hmac.HASH(token)
	err := first(ug.db.Where("remember_hash=?", rememberHash), &user)
	if err != nil {
		return nil, err
	}
	return &user, nil

}

//Authenticate can be used to authenticate the user with the provided email address and the password.
func (us *UserService) Authenticate(email, password string) (*User, error) {
	foundUser, err := us.ByEmail(email)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(foundUser.PasswordHash), []byte(password+userPwPepper))

	if err != nil {
		switch err {
		case bcrypt.ErrMismatchedHashAndPassword:
			return nil, ErrInvalidPassword
		default:
			return nil, err
		}

	}

	return foundUser, nil

}

//First will query using the provided gorm.DB and it will
//get the first item returned and place it into dst,
//if nothing is found in the query it will return ErrNotFound

func first(db *gorm.DB, dst interface{}) error {
	err := db.First(dst).Error
	if err == gorm.ErrRecordNotFound {
		return ErrNotFound
	}
	return err

}

//Create will create the provided user and backfill data
//like the id,created at,updated at,deleted at
func (ug *userGorm) Create(user *User) error {
	pwBytes := []byte(user.Password + userPwPepper)
	hashedBytes, err := bcrypt.GenerateFromPassword(pwBytes, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hashedBytes)
	user.Password = ""
	if user.Remember == "" {
		token, err := rand.RememberToken()
		if err != nil {
			return err
		}
		user.Remember = token
	}
	user.RememberHash = ug.hmac.HASH(user.Remember)

	return ug.db.Create(user).Error

}

//Update will update the provided user with all of the data
//in the provided user object
func (ug *userGorm) Update(user *User) error {
	if user.Remember != "" {
		user.RememberHash = ug.hmac.HASH(user.Remember)
	}
	return ug.db.Save(user).Error

}

//Delete will delete the user with the provided id
func (ug *userGorm) Delete(id uint) error {
	if id == 0 {
		return ErrInvalidID
	}
	user := User{Model: gorm.Model{ID: id}}
	return ug.db.Delete(&user).Error

}

//Close closes the user service database connection
func (ug *userGorm) Close() error {
	return ug.db.Close()

}

//DestructiveReset drops a table and rebuilds it
func (ug *userGorm) DestructiveReset() error {
	if err := ug.db.DropTableIfExists(&User{}).Error; err != nil {
		return err
	}
	return ug.AutoMigrate()
}

//AutoMigrate will attempt to automatically migrate the//users table
func (ug *userGorm) AutoMigrate() error {
	if err := ug.db.AutoMigrate(&User{}).Error; err != nil {
		return err
	}
	return nil

}

//User is
type User struct {
	gorm.Model
	Name         string
	Email        string `gorm:"not null;unique_index"`
	Password     string `gorm:"-"`
	PasswordHash string `gorm:"not null"`
	Remember     string `gorm:"-"`
	RememberHash string `gorm:"not null;unique_index"`
}
