const NOMAD_API = process.argv[2] || 'http://127.0.0.1:4646'
const deploy = async () => {    
    try{            
        const res = await fetch(`${NOMAD_API}/v1/job/hello-worlds`)
        if(res.ok){
            console.log(await res.json())
            return
        }else{
            console.log('not found')
        }
    }catch(e){
        console.log('fetch error')
        await new Promise(rs=>setTimeout(rs, 2000))
    }
}

deploy()