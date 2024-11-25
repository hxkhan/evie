package main

/* fn main() {
    var accounts = go fs.readFile("accounts.json")
    var logs = go fs.readFile("logs.json")

    echo accounts
    echo logs

    var id = await.any(accounts, logs)

    if (id == 0) echo "accounts.json has been loaded"
    if (id == 1) echo "logs.json has been loaded"
} */

fn main() {
    mario := fs.readFile("mario.jpg")
    pacman := fs.readFile("pacman.jpg")

    echo mario
    echo pacman

    mario = await mario
    pacman = await pacman

    //mario, pacman := await.all(fs.readFile("mario.jpg"), fs.readFile("pacman.jpg"))

    echo mario
    echo pacman

    echo "both files have been loaded"
}


    //mario, pacman = await.all (mario, pacman)


    /* tasks := await.group (mario, pacman)

    value, id := await tasks
    

    echo await.any (mario, pacman)
    
    asset, id := await.any (mario, pacman)
    switch (id) {
        case 0: echo "mario.jpg loaded first"
        case 1: echo "pacman.jpg loaded first"
    } 
    
    mario = await mario -> catch (e) {

    }*/
/* 
doSomething() -> catch(e) {

    }


    try {
        mario = await mario
    } catch(e) {
        
    } */

/* fn main() {
    var accounts = simulateLoadFile("accounts.json")
    var logs = simulateLoadFile("logs.json")

    echo accounts
    echo logs
} */

/* fn main() {
    var accounts = go simulateLoadFile("accounts.json")
    var logs = go simulateLoadFile("logs.json")

    echo accounts
    echo logs

    await.all(accounts, logs)

    echo accounts
    echo logs
}

// simulate loading big files
fn simulateLoadFile(fileName) {
    if (fileName == "accounts.json") {
        time.sleep(2500)
        return "A SHIT LOAD OF ACCOUNTS"
    } else if (fileName == "logs.json") {
        time.sleep(1000)
        return "A SHIT LOAD OF LOGS"
    }
} */