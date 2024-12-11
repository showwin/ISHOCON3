from fastapi import FastAPI

app = FastAPI()

@app.get("/")
def read_root():
    return {"Hello": "World"}

@app.get("/api/trains")
def read_root():
    return {"trains": [
        {
          "id": 1,
          "name": "Train 1",
          "availability": {
            "Arena->Bridge": "◯",
            "Bridge->Cave": "△",
            "Cave->Dock": "✗",
            "Dock->Edge": "◯",
            "Edge->Dock": "◯",
            "Dock->Cave": "△",
            "Cave->Bridge": "◯",
            "Bridge->Arena": "✗"
          }
        },
        {
          "id": 2,
          "name": "Train 2",
          "availability": {
            "Arena->Bridge": "◯",
            "Bridge->Cave": "△",
            "Cave->Dock": "✗",
            "Dock->Edge": "◯",
            "Edge->Dock": "◯",
            "Dock->Cave": "△",
            "Cave->Bridge": "◯",
            "Bridge->Arena": "✗"
          }
        },
        {
          "id": 3,
          "name": "Train 3",
          "availability": {
            "Arena->Bridge": "◯",
            "Bridge->Cave": "△",
            "Cave->Dock": "✗",
            "Dock->Edge": "◯",
            "Edge->Dock": "◯",
            "Dock->Cave": "△",
            "Cave->Bridge": "◯",
            "Bridge->Arena": "✗"
          }
        },
    ]}

@app.get("/api/stations")
def read_root():
    return {"stations": [
        {"id": 1, "name": "Arena"},
        {"id": 2, "name": "Bridge"},
        {"id": 3, "name": "Cave"},
        {"id": 4, "name": "Dock"},
        {"id": 5, "name": "Edge"},
    ]}


