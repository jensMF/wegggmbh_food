package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/TwiN/go-color"
	"golang.org/x/exp/slices"
	"golang.org/x/term"
)

var client http.Client

const dateForm = "2006-1-2"
const printDateFormat = "Mon. 02-01-2006"
const placeOrder = false
const cancelOrder = true

const baseurl = "https://www.wegggmbh.de/intern/index.php"
const action = "essen"

const schliesszeitenKiTa = "2022-7-25..2022-8-5 2022-9-27 2022-9-28 2022-10-11 2022-12-20..2023-1-2 2023-5-19 2023-10-2 2023-8-14..2023-8-25"

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Got error while creating cookie jar %s", err.Error())
	}
	client = http.Client{
		Jar: jar,
	}
}

func main() {
	// Login
	var username string
	for !isMailaddressValid(username) {
		fmt.Fprintf(os.Stderr, "Please enter your username: ")
		fmt.Scanln(&username)
		if !isMailaddressValid(username) {
			fmt.Fprint(os.Stderr, color.InRed("error: invalid username: "+username+"\n"))
		}
	}
	var password string
	for strings.TrimSpace(password) == "" {
		fmt.Fprintf(os.Stderr, "Please enter your password: ")
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatal(err)
			password = ""
		} else {
			password = string(bytePassword)
		}
	}
	fmt.Println("")
	data := url.Values{
		"login;req":    {username},
		"password;req": {password},
		"Anmelden":     {"Anmelden"},
	}

	_, err1 := client.PostForm(baseurl, data)
	if err1 != nil {
		log.Fatal(err1)
	}

	updatePlacedOrdersAndServerTime(false)

	reader := bufio.NewReader(os.Stdin)

	proceed := true

	for proceed {
		fmt.Println("\n 1. print placed orders\n", "2. add orders\n", "3. delete orders\n", "4. backup ordered dates", "q quit the program")
		fmt.Fprintf(os.Stderr, "What do you want to do? (1/2/3/4/q) ")
		option, _ := reader.ReadString('\n')
		switch strings.TrimSpace(option) {
		case "1":
			updatePlacedOrdersAndServerTime(false)
		case "2":
			proceed = placeOrCancelOrder(placeOrder, "Please enter the dates you want to place a food order: \n")
		case "3":
			proceed = placeOrCancelOrder(cancelOrder, "Please enter the dates you want to cancel the food order: \n")
		case "4":
			updatePlacedOrdersAndServerTime(true)
		case "q":
			proceed = false
		default:
		}
	}
}

func isMailaddressValid(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func placeOrCancelOrder(cancelOrder bool, message string) bool {
	for {
		placedOdersDates, serverTime := updatePlacedOrdersAndServerTime(false)
		cut_offDate := getClosingDate(serverTime)
		fmt.Fprint(os.Stderr, color.InPurple("\n enter dates in these formats:\t\t 2006-1-2 or 2006-01-02\n enter dates seperated by spaces:\t 2022-5-13 2022-06-08 2022-7-3\n enter ranges of dates like this:\t 2022-5-13..2022-6-3\n"), color.InBlue(" example for an input:\n\t 2022-5-13..2022-6-3 2022-06-08 2022-7-3 2022-7-5..2022-7-18\n"), " q:\t\t quit the program\n anything else:\t return to main menu\n")
		if cancelOrder {
			fmt.Fprint(os.Stderr, color.InYellow(message))
		} else {
			fmt.Fprint(os.Stderr, color.InGreen(message))
		}

		dates, status, err := getDatesInput(cut_offDate)
		if err != nil {
			if status == 2 {
				return false
			}
			if status != 0 {
				ok := YesNoPrompt("\nDo you want to return to the menu?", false)
				if ok {
					return true
				}
			}
		}

		closedKiTaDates := parseDateString(schliesszeitenKiTa, cut_offDate)
		dates, _ = filter(dates, closedKiTaDates, false)
		dates, datesStr := filter(dates, placedOdersDates, cancelOrder)

		var str string
		if cancelOrder {
			str = "cancel"
		} else {
			str = "place"
		}

		if len(dates) != 0 {
			if cancelOrder {
				fmt.Println(color.InBold("\n"+str+" dates:\n"), color.InYellow(datesStr))
			} else {
				fmt.Println(color.InBold("\n"+str+" dates:\n"), color.InGreen(datesStr))
			}
			ok := YesNoPrompt("Do you want to "+str+" orders for these dates?", true)
			if ok {
				var requests int
				fmt.Println(str + " order no.:")
				for index, date := range dates {
					params := "action=" + url.QueryEscape(action) + "&" + "date=" + url.QueryEscape(date.Format("2006-1-2"))
					path := fmt.Sprintf(baseurl+"?%s", params)
					_, err := client.Get(path)
					if err != nil {
						log.Fatal(err)
					}
					requests = index
					fmt.Print(index, " ")
				}
				fmt.Println("\nmodyfied orders:", requests)
				fmt.Println("")
			} else {
				ok := YesNoPrompt("Do you want to return to the menu?", false)
				if ok {
					return true
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "no valid dates to "+str+" an order\n")
		}
	}
}

func updatePlacedOrdersAndServerTime(createBackup bool) (placedOdersDates []time.Time, serverTime time.Time) {
	// request dates where food order is placed
	what := "getuserdates"

	params := "action=" + url.QueryEscape(action) + "&" +
		"what=" + url.QueryEscape(what)
	userdates_path := fmt.Sprintf(baseurl+"?%s", params)

	userdates, err2 := client.Get(userdates_path)
	if err2 != nil {
		log.Fatal(err2)
	}

	defer userdates.Body.Close()

	serverTime, err := http.ParseTime(userdates.Header["Date"][0])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(color.InCyan("Server Time:"), color.InCyan(serverTime.Format(time.RFC850)))

	body, err := ioutil.ReadAll(userdates.Body)
	if err != nil {
		log.Fatal(err)
	}

	if createBackup {
		err = ioutil.WriteFile("ordered_dates_backup_"+serverTime.Format("2006-01-02_15-04-05_MST")+".txt", body, 0777)
		if err != nil {
			log.Fatal(err)
		}
	}

	placedOdersSourceStr := strings.Split(strings.Trim(strings.ReplaceAll(string(body), "\"", ""), "[]"), ",")
	var placedOdersDatesStr string
	for _, dateStr := range placedOdersSourceStr {
		date, _ := time.Parse(dateForm, dateStr)
		placedOdersDates = append(placedOdersDates, date)
		if date.Weekday() == time.Friday {
			placedOdersDatesStr = placedOdersDatesStr + " " + date.Format(printDateFormat) + "\n"
		} else {
			placedOdersDatesStr = placedOdersDatesStr + " " + date.Format(printDateFormat)
		}
	}
	datePlaceholder := "... ... ... ..."
	fmt.Println(color.InBold("placed food orders:"))
	if placedOdersDates[0].Weekday() == time.Monday {
		fmt.Println(placedOdersDatesStr)
	} else if placedOdersDates[0].Weekday() == time.Tuesday {
		fmt.Println(color.InBlue(" "+datePlaceholder) + placedOdersDatesStr)
	} else if placedOdersDates[0].Weekday() == time.Wednesday {
		fmt.Println(color.InBlue(" "+datePlaceholder+" "+datePlaceholder) + placedOdersDatesStr)
	} else if placedOdersDates[0].Weekday() == time.Thursday {
		fmt.Println(color.InBlue(" "+datePlaceholder+" "+datePlaceholder+" "+datePlaceholder) + placedOdersDatesStr)
	} else if placedOdersDates[0].Weekday() == time.Friday {
		fmt.Println(color.InBlue(" "+datePlaceholder+" "+datePlaceholder+" "+datePlaceholder+" "+datePlaceholder) + placedOdersDatesStr)
	}
	return
}

func getDatesInput(closingDate time.Time) ([]time.Time, int, error) {
	reader := bufio.NewReader(os.Stdin)
	datesStr, err := reader.ReadString('\n')
	if err != nil {
		return nil, 1, err
	}
	if datesStr == "q\n" {
		return nil, 2, fmt.Errorf("quit called")
	}

	if len(datesStr) > 1 {
		dates := parseDateString(datesStr, closingDate)
		if len(dates) != 0 {
			return dates, 0, nil
		}
		return dates, 1, fmt.Errorf("no parsable dates")
	}
	return nil, 0, fmt.Errorf("no parsable dates")
}

func parseDateString(datesStr string, cut_offDate time.Time) (dates []time.Time) {
	for _, dateStr := range strings.Fields(datesStr) {
		if strings.Contains(dateStr, "..") {
			start, end, err := parseDateRangeEdges(dateStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error while parsing date range %s", err.Error())
			} else {
				for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
					if d.After(cut_offDate) && d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
						dates = Insert(dates, d)
					}
				}
			}
		} else {
			date, err := time.Parse(dateForm, dateStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error while parsing date %s", err.Error())
			} else if date.After(cut_offDate) && date.Weekday() != time.Saturday && date.Weekday() != time.Sunday && !slices.Contains(dates, date) {
				dates = Insert(dates, date)
			}
		}
	}
	return
}

func parseDateRangeEdges(dateRangeStr string) (time.Time, time.Time, error) {
	startEnd := strings.SplitN(dateRangeStr, "..", 2)
	date1, err1 := time.Parse(dateForm, startEnd[0])
	date2, err2 := time.Parse(dateForm, startEnd[1])
	if err1 != nil {
		return date1, date2, err1
	}
	if err2 != nil {
		return date1, date2, err2
	}
	if date2.After(date1) {
		return date1, date2, nil
	} else {
		return date2, date1, nil
	}
}

func filter(toFilter []time.Time, filter []time.Time, intersection bool) (out []time.Time, outStr string) {
	f := make(map[time.Time]struct{}, len(filter))
	for _, u := range filter {
		f[u] = struct{}{}
	}
	for i, u := range toFilter {
		if _, ok := f[u]; ok == intersection {
			out = append(out, u)
			if i == 0 {
				outStr = outStr + u.Format(printDateFormat)
			} else if u.Weekday() == time.Friday {
				outStr = outStr + " " + u.Format(printDateFormat) + "\n"
			} else {
				outStr = outStr + " " + u.Format(printDateFormat)
			}
		}
	}
	return
}

func getClosingDate(serverTime time.Time) time.Time {
	if serverTime.Weekday() == time.Sunday {
		return serverTime.AddDate(0, 0, 1)
	}
	if serverTime.Weekday() == time.Saturday {
		return serverTime.AddDate(0, 0, 2)
	}
	if serverTime.Hour() < 8 {
		return serverTime.AddDate(0, 0, -1)
	}
	if serverTime.Weekday() == time.Friday {
		return serverTime.AddDate(0, 0, 3)
	}
	return serverTime
}

func Insert(dateSlice []time.Time, dateToInsert time.Time) []time.Time {
	i := sort.Search(len(dateSlice), func(i int) bool { return dateSlice[i].Equal(dateToInsert) || dateSlice[i].After(dateToInsert) })
	dateSlice = append(dateSlice, dateToInsert)
	copy(dateSlice[i+1:], dateSlice[i:])
	dateSlice[i] = dateToInsert
	return dateSlice
}

func YesNoPrompt(label string, def bool) bool {
	choices := "Y/n"
	if !def {
		choices = "y/N"
	}

	r := bufio.NewReader(os.Stdin)
	var s string

	for {
		fmt.Fprintf(os.Stderr, "%s (%s) ", label, choices)
		s, _ = r.ReadString('\n')
		s = strings.TrimSpace(s)
		if s == "" {
			return def
		}
		s = strings.ToLower(s)
		if s == "y" || s == "yes" {
			return true
		}
		if s == "n" || s == "no" {
			return false
		}
	}
}
