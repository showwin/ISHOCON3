import os
import requests

host = os.getenv("ISHOCON_PAYMENT_HOST", "payment_app") # TODO: proper host
port = int(os.getenv("ISHOCON_PAYMENT_PORT", "8080")) # TODO: 443

def capture_payment(amount, token) -> tuple[int, dict]:
    res = requests.post(
        f"http://{host}:{port}/payments",
        json={"amount": amount, "global_payment_token": token},
    )
    return res.json()

def payment_app_initialize():
    res = requests.post(f"http://{host}:{port}/initialize")
    return res.status_code
