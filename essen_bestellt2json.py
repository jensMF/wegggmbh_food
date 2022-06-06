import requests as req
import json

url = 'https://www.wegggmbh.de/intern/index.php'
user_data = {'login;req':'Hier Benutzername eintragen', 'password;req':'Hier Passwort Eintragen', 'Anmelden':'Anmelden'}
date_params = {'action':'essen', 'what':'getuserdates'}
login = req.post(url, data = user_data)
ordered_dates = req.get(url, params = date_params, cookies = login.cookies)

def main() -> int:
	try:
		login.raise_for_status()
	except e:
		print(f'login failed with {e}')
		return 1
	try:
		ordered_dates.raise_for_status()
	except e:
		print(f'retrieving dates failed with {e}')
		return 1
	#TODO: Den Punk . ersetzen durch den Pfad zum Backup-Ordner, sonst wird das Backup dort erstellet, wo das Skript ausgeführt wird.
	with open(f'./essen_bestellt_am_{ordered_dates.headers["Date"]}.json', 'w') as json_file:
		json.dump(ordered_dates.json(), json_file)
	return 0

main()
