
const {MongoClient, Timestamp} = require('mongodb')
const {spawn} = require('child_process')
const path = require('path')
const os = require('os')
const fs = require('node:fs/promises')
const _ = require('underscore')
const nodes = process.argv.slice(2)
const getOplog =  async node => {
    const exit = Promise.withResolvers();
    const fetch = Promise.withResolvers();

    console.log("getoplog")
    const DB_PATH = getDbPath(node)
    console.log(DB_PATH)
    try{
        const stat = await fs.stat(DB_PATH)
        if (!stat.isDirectory()){
            throw 'not directory'
        }
    }catch(e){
        await fs.mkdir(DB_PATH)
    }
    const PORT = '2701'+node
    const proc = spawn('mongod.exe', ['--dbpath', DB_PATH, '--port', PORT, '--bind_ip', '10.10.11.1'], {cwd : path.join(process.cwd(), 'cots','mongo')})
        .on('close', code=>{
            console.log(node, 'exited with', code)
            if(code){
                exit.reject(code)
            }else{
                exit.resolve()
            }
        })
        .on('error', e=>{
            console.log('mongod proc err', e)
            exit.resolve(e)
        })
        .on('spawn', async ()=>{
            try{
                const url = `mongodb://10.10.11.1:2701${node}`
                const cli = new MongoClient(url, {
                    serverSelectionTimeoutMS: 30000,
                    directConnection: true
                })

                await cli.connect()
                console.log(node, 'connected')

                const adminUser = (await cli.db('admin').command({usersInfo: {user: 'adminUser', db: 'admin'}  })).users
                if (adminUser.length==0){
                    console.log(node, 'creating admin user')
                    const userResult = await cli.db('admin').command({
                        createUser: 'adminUser',
                        pwd: '123',
                        roles:[
                            { role: "root", db: "admin" },
                            'readWriteAnyDatabase',
                            'clusterAdmin'
                        ]
                    })
                    console.log(node, userResult)
                }else{
                    console.log(node, 'admin user exist')
                }

                for await (log of cli.db('local').collection('oplog.rs').find().sort({$natural: -1}).limit(1)){
                    const replSet = await cli.db('local').collection('system.replset').findOne({_id: 'rs0'})
                    return fetch.resolve({
                        node,
                        oplog: log.ts,
                        replSet,
                    })
                }
                console.log(node, 'no oplog')
                return fetch.resolve({
                    node,
                    oplog: new Timestamp(),
                    replSet: null
                })
            }catch(e){
                console.log(e)
                return fetch.resolve(new Timestamp())
            }finally{
                proc.kill('SIGINT')
            }
        })
    proc.stdout.on('data', data=>{
        //console.log(data.toString('utf-8'))
    })
    proc.stderr.on('data', data=>{
        //console.log(data.toString('utf-8'))
    })

    const [fetchResult] = await Promise.all([fetch.promise, exit.promise])
    return fetchResult
}
const getDbPath = node => {
    return path.join(os.tmpdir(), `mongo${node}`)
}
const dropDb = async node => {
    try{
        console.log('removing dbpath')
        const stat = await fs.stat(getDbPath(node))
        if (stat.isDirectory()){
            return fs.rmdir(getDbPath(node), {
                               recursive: true
                           })
        }
    }catch(e){
        //console.log('fstat error')
    }
    return fs.mkdir(getDbPath(node))
}

const connect = async (node, nodes) => {

    console.log('connecting to', node.node)
    const client = Promise.withResolvers();

    const DB_PATH = getDbPath(node.node)
    const PORT = '2701'+node.node
    const keyFilePath = path.join(os.tmpdir(), `mongo_key_${node.node}`)
    await fs.writeFile(keyFilePath, 'MONGOSECRET')
    const proc = spawn('mongod.exe', ['--dbpath', DB_PATH, '--port', PORT, '--bind_ip', '10.10.11.1', '--replSet', 'rs0', '--auth', '--keyFile', keyFilePath], {cwd : path.join(process.cwd(), 'cots','mongo')})
        .on('close', code=>{
            console.log(node.node, 'exited with', code)
        })
        .on('error', e=>{
            console.log('mongod proc err', e)
            client.reject(e)
        })
        .on('spawn', async ()=>{
            const url = `mongodb://adminUser:123@10.10.11.1:2701${node.node}`
            const cli = new MongoClient(url, {
                serverSelectionTimeoutMS: 30000,
                directConnection: true
            })
            try{

                const res = await cli.connect()
                console.log(node.node, 'connected')
                client.resolve(res)
            }catch(e){
                console.log(node.node, 'connect error', e.codeName)
                //client.reject(e)
            }
        })
    proc.stdout.on('data', data=>{
        //if (node.node === '3')
        //    console.log(data.toString('utf-8'))
    })
    proc.stderr.on('data', data=>{
        //console.log(data.toString('utf-8'))
    })
    return client.promise
}

const init = async (node, nodes, cli) => {
    console.log('initiating replicaset', nodes.map(node=>node.node))
    const members = nodes.map(node=>({_id: Number.parseInt(node.node), host: `10.10.11.1:2701${node.node}`}))
    console.log(members)
    const replConf = await cli.db('admin').command({replSetInitiate : {_id: 'rs0', members}})
    console.log(replConf)
    return cli
}
const reconf = async (node, nodes, cli) => {
    console.log('reconf replicaset', nodes.map(node=>node.node))
    const members = nodes.map(node=>({_id: Number.parseInt(node.node), host: `10.10.11.1:2701${node.node}`}))
    const {config} = await cli.db('admin').command({replSetGetConfig : 1})
    const result = await cli.db('admin').command({replSetReconfig : {...config, members}, force: true})
    console.log(members, result)
    return cli
}
Promise.all(nodes.map(node=>getOplog(node)))
    .then(async result=>{
        const nodes = result.sort((b,a)=>{
            if (a.oplog.t === b.oplog.t){
                if (a.oplog.i === b.oplog.i){
                    return Number.parseInt(a.node) - Number.parseInt(b.node)
                }else{
                    return a.oplog.i - b.oplog.i
                }
            }
            return a.oplog.t - b.oplog.t
        })
        .slice(0,3)
        const [primary] = nodes
        console.log('MONGO Replset', JSON.stringify(nodes, 5, '  '))
        await Promise.all(nodes.map(async (node, i) => {
        const orders = ['connect']
            if(i == 0) {
                //this primary
                if (node.replSet) {
                    console.log("make reconfg", node.replSet)
                    orders.push('reconf')
                }else {
                    console.log('make initiate')
                    orders.push('init')
                }
            }else {
                if (node.replSet && node.replSet?.settings?.replicaSetId !== primary?.settings?.replicaSetId) {
                    //drop db
                    //console.log(JSON.stringify(nodes))
                    //console.log('drop db')
                    //orders.splice(0, 0, 'drop')
                    //await dropDb(node.node)
                }
            }

            let chain = null
            console.log('ORders of', node.node, orders)
            for(const order of orders) {
                switch (order) {
                    case 'connect':
                        chain = await connect(node, nodes, chain)
                        break;
                    case 'init':
                        chain = await init(node, nodes, chain)
                        break;
                    case 'reconf':
                        chain = await reconf(node, nodes, chain)
                        break;
                    case 'drop':
                        chain = await dropDb(node.node, nodes, chain)
                        break;
                }
            }

            if(i == 0) {
                let count = 0
                for(;;) {
                    try{
                        const repls = await chain.db('admin').command({replSetGetStatus : 1})
                        console.log(repls.members.map(m=>({n:m._id, s:m.stateStr})))
                        if (repls.members.filter(m=>m.stateStr == 'PRIMARY').length>0) {
                            count++
                        }else{
                            count = 0
                        }
                        if (count > 5) {
                            console.log('configuration done')
                            process.exit(0)
                        }
                    }catch(e) {
                        console.log('status err', e.codeName)
                    }
                        await new Promise(rs=>setTimeout(rs, 1000))
                }
            }

        }))
        
    })

/*


const opLogs = nodes.map(async node => {

    const oplog = await getOplog(node)
    console.log("op",oplog)
    await new Promise(rs=>setTimeout(rs, 5000))
    process.exit()
    const url = `mongodb://10.10.11.1:2701${node}`
        console.log(node, url)
    const cli = new MongoClient(url, {
        serverSelectionTimeoutMS: 30000,
        directConnection: true
    })
    try{

        console.log('connecting')
        await cli.connect()
        console.log('connected')
        try{
            const {config} = await cli.db('admin').command({replSetGetConfig: 1})
            const resp = await cli.db('admin').command({replSetReconfig : {...config, members:[{_id:Number.parseInt(node), host:`10.10.11.1:2701${node}`}]}, force: true})
            console.log("standalone is ok")
        }catch(e){
            console.log('no replset')
        }
        for(let i=0;i<10;i++){
            console.log('try get to oplog')
            try{
                const findResult =  cli.db('local').collection('oplog.rs').find().sort({$natural:-1}).limit(1)
                for await (const doc of findResult) {
                console.log('oplog fetched')
                  return {node, ts: doc.ts, cli}
                }

            }catch(e){
                console.log('oplog get failed', e)
                await new Promise(rs=>setTimeout(rs, 3000))
            }
        }
    }catch(e){
        console.log('error', e)
        return new {node, ts: new Timestamp(), cli}
    }
})

Promise.all(opLogs).then(async res=>{
    const sorted = res.sort((a,b)=>{
            if(a.ts.t === b.ts.t){
                return a.ts.i - b.ts.i
            }
            return a.ts.t - b.ts.t
        })

    const [target] = sorted
    console.log("active member", target.node)
    console.log('getting current config')
    const {config} = await target.cli.db('admin').command({replSetGetConfig: 1})
    const members = sorted.map(s=>({_id: Number.parseInt(s.node), host:`10.10.11.1:2701${s.node}`}))
    console.log(members)
    try{

        const resp = await target.cli.db('admin').command({replSetReconfig : {...config, members}, force: true})
    }catch(e){
        console.log('reconf failed', e)
    }
    for(;;){
            const stat = await target.cli.db('admin').command({replSetGetStatus: 1})
            if (stat.members.map(s=>s.stateStr).indexOf('PRIMARY')>=0){
                console.log('PRIMARY OK')
                break;
            }
            console.log(stat.members.map(s=>s.stateStr))
            await new Promise(rs=>setTimeout(rs, 1000))
    }
    for(let i=0;i<10;i++) {
        try{
            const stat = await target.cli.db('admin').command({replSetGetStatus: 1})
            console.log(stat.members.map(s=>s.stateStr))
        }catch(e){
            console.log('stat error')
        }
            await new Promise(rs=>setTimeout(rs, 1000))
    }
    process.exit(0)
})


*/

