// @ts-nocheck
import { useAtomValue } from 'jotai'

import { useNavigate, useSearchParams } from 'react-router-dom'
import { ChevronRightIcon } from '@heroicons/react/24/solid'
import { GithubComKaytuIoKaytuEnginePkgComplianceApiBenchmarkEvaluationSummary } from '../../../../api/api'
import { benchmarkChecks } from './SeverityBar'
import SummaryCard from '../../../../components/Cards/SummaryCard'
import {
    getConnectorIcon,
    getConnectorsIcon,
} from '../../../../components/Cards/ConnectorCard'
import SeverityBar from './SeverityBar'
import { searchAtom } from '../../../../utilities/urlstate'
import { isDemoAtom } from '../../../../store'
import Cards from '@cloudscape-design/components/cards'
import Box from '@cloudscape-design/components/box'
import SpaceBetween from '@cloudscape-design/components/space-between'
import Button from '@cloudscape-design/components/button'
import Header from '@cloudscape-design/components/header'
import { Link } from '@cloudscape-design/components'
import Badge from '@cloudscape-design/components/badge'
import { Flex } from '@tremor/react'

interface IComplianceCard {
    benchmark: NewBenchmark[] | undefined
    all: any[]
    loading: boolean
}
export interface NewBenchmark {
    benchmark_id: string
    compliance_score: number
    severity_summary_by_control: SeveritySummaryBy
    severity_summary_by_resource: SeveritySummaryBy
    findings_summary: FindingsSummary
    issues_count: number
    top_integrations: null
    top_resources_with_issues: TopSWithIssue[]
    top_resource_types_with_issues: TopSWithIssue[]
    top_controls_with_issues: TopSWithIssue[]
    last_evaluated_at: Date
    last_job_status: string
    last_job_id: string
}

export interface FindingsSummary {
    total_count: number
    passed: number
    failed: number
}

export interface SeveritySummaryBy {
    total: Critical
    critical: Critical
    high: Critical
    medium: Critical
    low: Critical
    none: Critical
}

export interface Critical {
    total: number
    passed: number
    failed: number
}

export interface TopSWithIssue {
    field: Field
    key: string
    issues: number
}

export enum Field {
    Control = 'Control',
    Resource = 'Resource',
    ResourceType = 'ResourceType',
}
//    <SeverityBar benchmark={benchmark} />
export default function BenchmarkCards({ benchmark, all,loading }: IComplianceCard) {
    const navigate = useNavigate()
    const searchParams = useAtomValue(searchAtom)
    const isDemo = useAtomValue(isDemoAtom)
    const truncate = (text: string | undefined) => {
        if (text) {
            return text.length > 25 ? text.substring(0, 25) + '...' : text
        }
        else{
            return '...'
        }
    }
    return (
        <>
            <Cards
                className='w-full'
                ariaLabels={{
                    itemSelectionLabel: (e, t) => `select ${t.name}`,
                    selectionGroupLabel: 'Item selection',
                }}
                cardDefinition={{
                    header: (item) => (
                        <Link
                            className="mb-10"
                            onClick={(e) => {
                                e.preventDefault()
                                // console.log(item.id)
                                // navigate(`${item.id}`)
                            }}
                            href={`./compliance/${item.id}`}
                            fontSize="heading-m"
                        >
                            <Flex className="w-100" justifyContent="between" alignItems='center'>
                                <Flex className='w-100 min-w-max' justifyContent='start'>{truncate(item.name)}</Flex>
                                <Flex justifyContent="end" className="gap-2">
                                    {item?.connectors?.map((sub) => {
                                        return (
                                            <>
                                                <Badge>{sub}</Badge>
                                            </>
                                        )
                                    })}
                                </Flex>
                            </Flex>
                        </Link>
                    ),
                    sections: [
                        {
                            id: 'security_score',
                            header: '',
                            content: (item) => '',
                        },

                        {
                            id: 'security_score',
                            header: 'Compliance Score',
                            content: (item) => `${item.security_score}%`,
                        },
                        {
                            id: 'Severity',
                            header: 'Severity',
                            content: (item) => {
                                return (
                                    <SeverityBar benchmark={item.benchmark} />
                                )
                            },
                        },
                    ],
                }}
                cardsPerRow={[{ cards: 2 }]}
                // totalItemsCount={7}
                items={benchmark?.map((item) => {
                    return {
                        name: item?.benchmark_title,
                        benchmark: item,
                        security_score: (item?.compliance_score * 100).toFixed(
                            0
                        ),
                        id: item.benchmark_id,
                        connectors: item.connectors,
                    }
                })}
                entireCardClickable
                loadingText="Loading resources"
                loading={loading}
                empty={
                    <Box
                        margin={{ vertical: 'xs' }}
                        textAlign="center"
                        color="inherit"
                    >
                        <SpaceBetween size="m">
                            <b>No resources</b>
                            {/* <Button>Create resource</Button> */}
                        </SpaceBetween>
                    </Box>
                }
                // header={<Header>Example Cards</Header>}
            />
        </>
    )
}
