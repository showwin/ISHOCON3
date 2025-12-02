import json
from decimal import Decimal
from datetime import datetime

import boto3

client = boto3.client("dynamodb")
dynamodb = boto3.resource("dynamodb")
table = dynamodb.Table("${dynamodb_table_name}")
scoreboard_closed_at_key = "scoreboard_closed_at"

def lambda_handler(event, context):
    print(event)
    body = {}
    statusCode = 200
    headers = {"Content-Type": "application/json"}

    try:
        if event["routeKey"] == "DELETE /teams":
            scan = table.scan()
            with table.batch_writer() as batch:
                for each in scan["Items"]:
                    batch.delete_item(
                        Key={"team": each["team"], "timestamp": each["timestamp"]}
                    )
            body = "Deleted all items"
        elif event["routeKey"] == "GET /teams":
            body = table.scan()
            body = body["Items"]
            responseBody = []
            for items in body:
                # Exclude scoreboard_closed_at record
                if items["team"] == scoreboard_closed_at_key:
                    continue
                responseItems = {
                    "score": float(items["score"]),
                    "team": items["team"],
                    "timestamp": items["timestamp"],
                }
                responseBody.append(responseItems)
            body = responseBody
        elif event["routeKey"] == "PUT /teams":
            requestJSON = json.loads(event["body"])
            table.put_item(
                Item={
                    "team": requestJSON["team"],
                    "score": Decimal(str(requestJSON["score"])),
                    "timestamp": requestJSON["timestamp"],
                }
            )
            body = "Put item " + requestJSON["team"]
        elif event["routeKey"] == "POST /scoreboard/closed_at":
            timestamp = datetime.now().isoformat()
            table.put_item(
                Item={
                    "team": scoreboard_closed_at_key,
                    "score": Decimal("0"),
                    "timestamp": timestamp,
                }
            )
            body = {"timestamp": timestamp}
        elif event["routeKey"] == "GET /scoreboard/closed_at":
            response = table.query(
                KeyConditionExpression=boto3.dynamodb.conditions.Key("team").eq(scoreboard_closed_at_key)
            )
            if response["Items"]:
                body = {"timestamp": response["Items"][0]["timestamp"]}
            else:
                statusCode = 404
                body = "The scoreboard is not yet closed"
        elif event["routeKey"] == "DELETE /scoreboard/closed_at":
            response = table.query(
                KeyConditionExpression=boto3.dynamodb.conditions.Key("team").eq(scoreboard_closed_at_key)
            )
            if response["Items"]:
                with table.batch_writer() as batch:
                    for each in response["Items"]:
                        batch.delete_item(
                            Key={"team":scoreboard_closed_at_key, "timestamp": each["timestamp"]}
                        )
                body = "Deleted scoreboard_closed_at"
            else:
                statusCode = 404
                body = "The scoreboard is not yet closed"
    except KeyError:
        statusCode = 400
        body = "Unsupported route: " + event["routeKey"]
    body = json.dumps(body)
    res = {
        "statusCode": statusCode,
        "headers": {"Content-Type": "application/json"},
        "body": body,
    }
    return res
