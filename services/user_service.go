package services

import (
	"context"
	"fmt"
	"os"
	handler "technical-test/priverion/handlers"
	"technical-test/priverion/models"
	"technical-test/priverion/utils"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	db         *utils.Database
	collection *mongo.Collection
}

func NewUserService(db *utils.Database, dbName string, col string) *UserService {
	return &UserService{
		db:         db,
		collection: db.Client.Database(dbName).Collection(col),
	}
}

func (us *UserService) SignUp(ctx *gin.Context) (models.User, error) {
	var user models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		return user, err
	}
	// validation of user fields
	if errFields := handler.SignUpValidation(ctx, &user); errFields != nil {
		return user, errFields
	}
	// verify if the email already exist in the database
	res := us.collection.FindOne(context.TODO(), bson.M{"email": user.Email})
	if res.Err() == nil {
		return user, fmt.Errorf("email already exists")
	} else if res.Err() != mongo.ErrNoDocuments {
		return user, res.Err()
	}
	// encrypt the password
	paswordHashed, err := hashPassword(user.Password)
	if err != nil {
		return user, err
	}

	user.ID = primitive.NewObjectID()
	user.CreatedAt = time.Now()
	user.Password = paswordHashed

	// save user to the database
	_, err = us.collection.InsertOne(context.TODO(), user)
	if err != nil {
		return user, err
	}

	return user, nil
}

func (us *UserService) LogIn(ctx *gin.Context) (models.JWTOutput, error) {
	var user models.User
	var foundUser models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		return models.JWTOutput{}, err
	}
	// Verify email
	err := us.collection.FindOne(context.TODO(), bson.M{"email": user.Email}).Decode(&foundUser)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// User with the provided email not found
			return models.JWTOutput{}, fmt.Errorf("user not found")
		}
		return models.JWTOutput{}, err
	}

	// Verify password
	passwordIsValid := verifyPassword(user.Password, foundUser.Password)
	if !passwordIsValid {
		// Incorrect password
		return models.JWTOutput{}, fmt.Errorf("incorrect password")
	}

	token, errJWT := us.GenerateJWT(foundUser)
	// Check for JWT token generation errors
	if errJWT != nil {
		return models.JWTOutput{}, errJWT
	}

	return token, nil
}

// update user role by id
func (us *UserService) UpdateRoleUser(id string, userUpdated models.User) (int, error) {
	objectID, errParse := primitive.ObjectIDFromHex(id)
	if errParse != nil {
		return 0, errParse
	}

	filter := bson.M{"_id": objectID}
	update := bson.D{{
		Key: "$set", Value: bson.D{
			{Key: "roles", Value: userUpdated.Roles},
		},
	}}
	result, err := us.collection.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		return 0, fmt.Errorf("could not update user role: %w", err)
	}

	// to make sure it was modified the document
	if result.ModifiedCount == 0 {
		return 0, fmt.Errorf("user not found or not updated")
	}

	return int(result.ModifiedCount), nil
}

func (us *UserService) GenerateJWT(user models.User) (models.JWTOutput, error) {
	// expiration time by token
	expirationTime := time.Now().Add(60 * time.Minute)

	// claims to the generate token
	claims := &models.Claims{
		Username: user.Username,
		Roles:    user.Roles,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}

	// generating token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(os.Getenv("SECRET_KEY")))
	if err != nil {
		return models.JWTOutput{}, err
	}

	// fill out the jwt output response
	jwtOutput := models.JWTOutput{
		Token:   tokenString,
		Expires: expirationTime,
	}
	return jwtOutput, nil
}

func hashPassword(password string) (string, error) {
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", err
	}
	return string(hashedPwd), nil
}

func verifyPassword(userPassword string, providedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(providedPassword), []byte(userPassword))
	check := true

	if err != nil {
		check = false
	}

	return check
}
