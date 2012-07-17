package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/timeredbull/tsuru/config"
	"github.com/timeredbull/tsuru/db"
	"github.com/timeredbull/tsuru/errors"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"
)

type isInGitosisChecker struct{}

func (c *isInGitosisChecker) Info() *CheckerInfo {
	return &CheckerInfo{Name: "IsInGitosis", Params: []string{"str"}}
}

func (c *isInGitosisChecker) Check(params []interface{}, names []string) (bool, string) {
	if len(params) != 1 {
		return false, "you should provide one string parameter"
	}
	str, ok := params[0].(string)
	if !ok {
		return false, "the parameter should be a string"
	}
	gitosisRepo, err := config.GetString("git:gitosis-repo")
	if err != nil {
		return false, "failed to get config"
	}
	path := path.Join(gitosisRepo, "gitosis.conf")
	f, err := os.Open(path)
	if err != nil {
		return false, err.Error()
	}
	defer f.Close()
	content, err := ioutil.ReadAll(f)
	if err != nil {
		return false, err.Error()
	}
	return strings.Contains(string(content), str), ""
}

var IsInGitosis, NotInGitosis Checker = &isInGitosisChecker{}, Not(IsInGitosis)

func (s *S) TestCreateUserHandlerSavesTheUserInTheDatabase(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, IsNil)

	u := User{Email: "nobody@globo.com"}
	err = u.Get()
	c.Assert(err, IsNil)
}

func (s *S) TestCreateUserHandlerReturnsStatus201AfterCreateTheUser(c *C) {
	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, IsNil)
	c.Assert(recorder.Code, Equals, 201)
}

func (s *S) TestCreateUserHandlerReturnErrorIfReadingBodyFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	request.Body.Close()
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^.*bad file descriptor$")
}

func (s *S) TestCreateUserHandlerReturnErrorAndBadRequestIfInvalidJSONIsGiven(c *C) {
	b := bytes.NewBufferString(`["invalid json":"i'm invalid"]`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^invalid character.*$")

	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestCreateUserHandlerReturnErrorAndConflictIfItFailsToCreateUser(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"email":"nobody@globo.com","password":"123"}`)
	request, err := http.NewRequest("POST", "/users", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateUser(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "This email is already registered")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *S) TestLoginShouldCreateTokenInTheDatabaseAndReturnItWithinTheResponse(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"password":"123"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, IsNil)

	var user User
	collection := db.Session.Users()
	err = collection.Find(bson.M{"email": "nobody@globo.com"}).One(&user)

	var recorderJson map[string]string
	r, _ := ioutil.ReadAll(recorder.Body)
	json.Unmarshal(r, &recorderJson)
	c.Assert(recorderJson["token"], Equals, user.Tokens[0].Token)
}

func (s *S) TestLoginShouldReturnErrorAndBadRequestIfItReceivesAnInvalidJson(c *C) {
	b := bytes.NewBufferString(`"invalid":"json"]`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Invalid JSON$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestLoginShouldReturnErrorAndBadRequestIfTheJSONDoesNotContainsAPassword(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide a password to login$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestLoginShouldReturnErrorAndNotFoundIfTheUserDoesNotExist(c *C) {
	b := bytes.NewBufferString(`{"password":"123"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^User not found$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *S) TestLoginShouldreturnErrorIfThePasswordDoesNotMatch(c *C) {
	u := User{Email: "nobody@globo.com", Password: "123"}
	u.Create()

	b := bytes.NewBufferString(`{"password":"1234"}`)
	request, err := http.NewRequest("POST", "/users/nobody@globo.com/tokens?:email=nobody@globo.com", b)
	c.Assert(err, IsNil)

	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^Authentication failed, wrong password$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
}

func (s *S) TestLoginShouldReturnErrorAndInternalServerErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = Login(recorder, request)
	c.Assert(err, NotNil)
}

func (s *S) TestCreateTeamHandlerSavesTheTeamInTheDatabaseWithTheAuthenticatedUser(c *C) {
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, IsNil)

	t := new(Team)
	err = db.Session.Teams().Find(bson.M{"name": "timeredbull"}).One(t)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, s.user)
}

func (s *S) TestCreateTeamHandlerReturnsBadRequestIfTheRequestBodyIsAnInvalidJSON(c *C) {
	b := bytes.NewBufferString(`{"name"["invalidjson"]}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestCreateTeamHandlerReturnsBadRequestIfTheNameIsNotGiven(c *C) {
	b := bytes.NewBufferString(`{"genre":"male"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	c.Assert(err, ErrorMatches, "^You must provide the team name$")
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
}

func (s *S) TestCreateTeamHandlerReturnsInternalServerErrorIfReadAllFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	err := b.Close()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestCreateTeamHandlerReturnConflictIfTheTeamToBeCreatedAlreadyExists(c *C) {
	err := db.Session.Teams().Insert(bson.M{"name": "timeredbull"})
	c.Assert(err, IsNil)
	b := bytes.NewBufferString(`{"name":"timeredbull"}`)
	request, err := http.NewRequest("POST", "/teams", b)
	c.Assert(err, IsNil)
	request.Header.Set("Content-type", "application/json")
	recorder := httptest.NewRecorder()
	err = CreateTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
	c.Assert(e, ErrorMatches, "^This team already exists$")
}

func (s *S) TestCreateTeamCreatesTheGroupWithinGitosis(c *C) {
	err := createTeam("timeredbull", s.user)
	time.Sleep(1e9)
	c.Assert(err, IsNil)
	c.Assert("[group timeredbull]", IsInGitosis)
}

func (s *S) TestCreateTeamAddsTheLoggedInUserKeysAsMemberInGitosis(c *C) {
	err := addKeyToUser("my-key", s.user)
	c.Assert(err, IsNil)
	keyname := s.user.Keys[0].Name
	err = createTeam("timeredbull", s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("[group timeredbull]", IsInGitosis)
	c.Assert("members = "+keyname, IsInGitosis)
}

func (s *S) TestAddUserToTeamShouldAddAUserToATeamIfTheUserAndTheTeamExistAndTheGivenUserIsMemberOfTheTeam(c *C) {
	u := &User{Email: "wolverine@xmen.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	url := "/teams/cobrateam/wolverine@xmen.com?:team=cobrateam&:user=wolverine@xmen.com"
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, IsNil)
	t := new(Team)
	err = db.Session.Teams().Find(bson.M{"name": "cobrateam"}).One(t)
	c.Assert(err, IsNil)
	c.Assert(t, ContainsUser, s.user)
	c.Assert(t, ContainsUser, u)
}

func (s *S) TestAddUserToTeamShouldReturnNotFoundIfThereIsNoTeamWithTheGivenName(c *C) {
	request, err := http.NewRequest("PUT", "/teams/abc/me@me.me?:team=abc&:user=me@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *S) TestAddUserToTeamShouldReturnUnauthorizedIfTheGivenUserIsNotInTheGivenTeam(c *C) {
	u := &User{Email: "hi@me.me", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	request, err := http.NewRequest("PUT", "/teams/cobrateam/hi@me.me?:team=cobrateam&:user=hi@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, u)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^You are not authorized to add new users to the team cobrateam$")
}

func (s *S) TestAddUserToTeamShouldReturnNotFoundIfTheEmailInTheBodyDoesNotExistInTheDatabase(c *C) {
	request, err := http.NewRequest("PUT", "/teams/cobrateam/hi2@me.me?:team=cobrateam&:user=hi2@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^User not found$")
}

func (s *S) TestAddUserToTeamShouldReturnConflictIfTheUserIsAlreadyInTheGroup(c *C) {
	url := fmt.Sprintf("/teams/%s/%s?:team=%s&:user=%s", s.team.Name, s.user.Email, s.team.Name, s.user.Email)
	request, err := http.NewRequest("PUT", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddUserToTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *S) TestAddUserToTeamShoulAddAllUsersKeyToGitosisConf(c *C) {
	u := &User{Email: "marathon@rush.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	s.addGroup()
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	err = addUserToTeam("marathon@rush.com", s.team.Name, s.user)
	c.Assert(err, IsNil)
	keyname := u.Keys[0].Name
	time.Sleep(1e9)
	c.Assert("members = "+keyname, IsInGitosis)
}

func (s *S) TestRemoveUserFromTeamShouldRemoveAUserFromATeamIfTheTeamExistAndTheUserIsMemberOfTheTeam(c *C) {
	u := User{Email: "nonee@me.me", Password: "none"}
	err := u.Create()
	c.Assert(err, IsNil)
	s.team.addUser(&u)
	db.Session.Teams().Update(bson.M{"name": s.team.Name}, s.team)
	request, err := http.NewRequest("DELETE", "/teams/cobrateam/nonee@me.me?:team=cobrateam&:user=nonee@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, IsNil)

	team := new(Team)
	err = db.Session.Teams().Find(bson.M{"name": s.team.Name}).One(team)
	c.Assert(err, IsNil)
	c.Assert(team, Not(ContainsUser), &u)
}

func (s *S) TestRemoveUserFromTeamShouldReturnNotFoundIfTheTeamDoesNotExist(c *C) {
	request, err := http.NewRequest("DELETE", "/teams/cobrateam/none@me.me?:team=unknown&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
	c.Assert(e, ErrorMatches, "^Team not found$")
}

func (s *S) TestRemoveUserfromTeamShouldReturnUnauthorizedIfTheGivenUserIsNotMemberOfTheTeam(c *C) {
	request, err := http.NewRequest("DELETE", "/teams/cobrateam/none@me.me?:team=cobrateam&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, &User{Email: "unknown@gmail.com"})
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusUnauthorized)
	c.Assert(e, ErrorMatches, "^You are not authorized to remove a member from the team cobrateam")
}

func (s *S) TestRemoveUserFromTeamShouldReturnNotFoundWhenTheUserIsNotMemberOfTheTeam(c *C) {
	u := &User{Email: "nobody@me.me", Password: "132"}
	s.team.addUser(u)
	db.Session.Teams().Update(bson.M{"name": s.team.Name}, s.team)
	defer func(t *Team, u *User) {
		s.team.removeUser(u)
		db.Session.Teams().Update(bson.M{"name": t.Name}, t)
	}(s.team, u)
	request, err := http.NewRequest("DELETE", "/teams/cobrateam/none@me.me?:team=cobrateam&:user=none@me.me", nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *S) TestRemoveUserFromTeamShouldReturnForbiddenIfTheUserIsTheLastInTheTeam(c *C) {
	url := "/teams/cobrateam/timeredbull@globo.com?:team=cobrateam&:user=timeredbull@globo.com"
	request, err := http.NewRequest("DELETE", url, nil)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveUserFromTeam(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusForbidden)
	c.Assert(e, ErrorMatches, "^You can not remove this user from this team, because it is the last user within the team, and a team can not be orphaned$")
}

func (s *S) TestRemoveUserFromTeamRemovesTheMemberFromGroupInGitosis(c *C) {
	u := &User{Email: "pomar@nando-reis.com", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	defer db.Session.Users().Remove(bson.M{"email": u.Email})
	s.addGroup()
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	err = u.Get()
	c.Assert(err, IsNil)
	err = addUserToTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	keyname := u.Keys[0].Name
	err = removeUserFromTeam("pomar@nando-reis.com", s.team.Name, s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("members = "+keyname, NotInGitosis)
}

func (s *S) TestAddKeyHandlerAddsAKeyToTheUser(c *C) {
	defer func() {
		s.user.removeKey(Key{Content: "my-key"})
		db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, IsNil)
	s.user.Get()
	c.Assert(s.user, HasKey, "my-key")
}

func (s *S) TestAddKeyHandlerReturnsErrorIfTheReadingOfTheBodyFails(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("POST", "/users/keys", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestAddKeyHandlerReturnsBadRequestIfTheJsonIsInvalid(c *C) {
	b := bytes.NewBufferString(`"aaaa}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Invalid JSON$")
}

func (s *S) TestAddKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *S) TestAddKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *S) TestAddKeyHandlerReturnsConflictIfTheKeyIsAlreadyPresent(c *C) {
	s.user.addKey(Key{Content: "my-key"})
	db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	defer func() {
		s.user.removeKey(Key{Content: "my-key"})
		db.Session.Users().Update(bson.M{"email": s.user.Email}, s.user)
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("POST", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = AddKeyToUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusConflict)
}

func (s *S) TestAddKeyFunctionCreatesTheKeyFileInTheGitosisRepository(c *C) {
	u := &User{Email: "francisco@franciscosouza.net", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	defer func() {
		removeKeyFromUser("my-key", u)
		db.Session.Users().RemoveAll(bson.M{"email": u.Email})
	}()
	c.Assert(u.Keys[0].Name, Not(Matches), "\\.pub$")
	keypath := path.Join(s.gitosisRepo, "keydir", u.Keys[0].Name+".pub")
	_, err = os.Stat(keypath)
	c.Assert(err, IsNil)
}

func (s *S) TestAddKeyFunctionAddTheMemberWithTheKeyNameInTheGitosisConfigurationFileInAllTeamsThatTheUserIsMember(c *C) {
	s.addGroup()
	err := addKeyToUser("my-key", s.user)
	c.Assert(err, IsNil)
	defer func() {
		removeKeyFromUser("my-key", s.user)
		time.Sleep(1e9)
	}()
	keyname := s.user.Keys[0].Name

	ch := make(chan int8)
	go func(ch chan int8, k string) {
		for !c.Check("members = "+keyname, IsInGitosis) {
		}
		ch <- 1
	}(ch, keyname)
	select {
	case <-ch:
		c.SucceedNow()
	case <-time.After(10 * time.Second):
		c.Error("Did not add the key in gitosis file in a suitable time.")
	}
}

func (s *S) TestRemoveKeyHandlerRemovesTheKeyFromTheUser(c *C) {
	s.addGroup()
	addKeyToUser("my-key", s.user)
	defer func() {
		if s.user.hasKey(Key{Content: "my-key"}) {
			removeKeyFromUser("my-key", s.user)
		}
	}()
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, IsNil)
	s.user.Get()
	c.Assert(s.user, Not(HasKey), "my-key")
}

func (s *S) TestRemoveKeyHandlerReturnsErrorInCaseOfAnyIOFailure(c *C) {
	b := s.getTestData("bodyToBeClosed.txt")
	b.Close()
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
}

func (s *S) TestRemoveKeyHandlerReturnsBadRequestIfTheJSONIsInvalid(c *C) {
	b := bytes.NewBufferString(`invalid"json}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Invalid JSON$")
}

func (s *S) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsNotPresent(c *C) {
	b := bytes.NewBufferString(`{}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *S) TestRemoveKeyHandlerReturnsBadRequestIfTheKeyIsEmpty(c *C) {
	b := bytes.NewBufferString(`{"key":""}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusBadRequest)
	c.Assert(e, ErrorMatches, "^Missing key$")
}

func (s *S) TestRemoveKeyHandlerReturnsNotFoundIfTheUserDoesNotHaveTheKey(c *C) {
	b := bytes.NewBufferString(`{"key":"my-key"}`)
	request, err := http.NewRequest("DELETE", "/users/key", b)
	c.Assert(err, IsNil)
	recorder := httptest.NewRecorder()
	err = RemoveKeyFromUser(recorder, request, s.user)
	c.Assert(err, NotNil)
	e, ok := err.(*errors.Http)
	c.Assert(ok, Equals, true)
	c.Assert(e.Code, Equals, http.StatusNotFound)
}

func (s *S) TestRemoveKeyHandlerDeletesTheKeyFileFromTheKeydir(c *C) {
	u := &User{Email: "francisco@franciscosouza.net", Password: "123"}
	err := u.Create()
	c.Assert(err, IsNil)
	err = addKeyToUser("my-key", u)
	c.Assert(err, IsNil)
	keypath := path.Join(s.gitosisRepo, "keydir", u.Keys[0].Name+".pub")
	err = removeKeyFromUser("my-key", u)
	c.Assert(err, IsNil)

	// Failing the test if the file is not deleted after 10 seconds
	ch := make(chan error)
	go func(c chan error, kp string) {
		var e error
		for _, e = os.Stat(kp); e == nil; _, e = os.Stat(kp) {
		}
		c <- e
	}(ch, keypath)
	select {
	case err = <-ch:
		c.Assert(os.IsNotExist(err), Equals, true)
	case <-time.After(10 * time.Second):
		c.Error("Did not delete the key file in a suitable time.")
	}
}

func (s *S) TestRemoveKeyHandlerRemovesTheMemberEntryFromGitosis(c *C) {
	err := addKeyToUser("my-key", s.user)
	c.Assert(err, IsNil)
	keyname := s.user.Keys[0].Name
	err = removeKeyFromUser("my-key", s.user)
	c.Assert(err, IsNil)
	time.Sleep(1e9)
	c.Assert("members = "+keyname, NotInGitosis)
}
