import os
import requests

from pydantic import BaseModel

host = os.getenv("ISHOCON_PAYMENT_HOST", "payment_app")
port = int(os.getenv("ISHOCON_PAYMENT_PORT", "8080"))

class PaymentAppResponse(BaseModel):
    status: str
    message: str

def capture_payment(amount, token) -> PaymentAppResponse:
    res = requests.post(
        f"http://{host}:{port}/payments",
        json={"amount": amount, "global_payment_token": token},
    )
    return PaymentAppResponse(**res.json())

def payment_app_initialize():
    res = requests.post(f"http://{host}:{port}/initialize")
    return res.status_code
